/*
 * The MIT License (MIT)
 *
 * Copyright (c) 2012-2014  Dustin Sallings <dustin@spy.net>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 * <http://www.opensource.org/licenses/mit-license.php>
 */

// Package nntpclient provides an NNTP Client.
package nntpclient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/kothawoc/go-nntp"
)

// Client is an NNTP client.
type Client struct {
	conn         *textproto.Conn
	netconn      net.Conn
	tls          bool
	Banner       string
	capabilities []string
}

// New connects a client to an NNTP server.
func New(net, addr string) (*Client, error) {
	conn, err := textproto.Dial(net, addr)
	if err != nil {
		return nil, err
	}

	_, msg, err := conn.ReadCodeLine(200)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		Banner: msg,
	}, nil
}

// New connects a client to an NNTP server.
func NewConn(establishedConn io.ReadWriteCloser) (*Client, error) {
	conn := textproto.NewConn(establishedConn)

	_, msg, err := conn.ReadCodeLine(200)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		Banner: msg,
	}, nil
}

// Authenticate against an NNTP server using authinfo user/pass
func (c *Client) Authenticate(user, pass string) (msg string, err error) {
	err = c.conn.PrintfLine("authinfo user %s", user)
	if err != nil {
		return
	}
	_, _, err = c.conn.ReadCodeLine(381)
	if err != nil {
		return
	}

	err = c.conn.PrintfLine("authinfo pass %s", pass)
	if err != nil {
		return
	}
	_, msg, err = c.conn.ReadCodeLine(281)
	return
}

func parsePosting(p string) nntp.PostingStatus {
	switch p {
	case "y":
		return nntp.PostingPermitted
	case "m":
		return nntp.PostingModerated
	}
	return nntp.PostingNotPermitted
}

// List groups
func (c *Client) List(sub string) (rv []nntp.Group, err error) {
	rv = make([]nntp.Group, 0)
	if sub != "" {
		sub = " " + sub
	}
	_, _, err = c.Command("LIST"+sub, 215)
	if err != nil {
		slog.Error("list failed, abandoning, error", "error", err)
		return
	}
	var groupLines []string
	groupLines, err = c.conn.ReadDotLines()
	if err != nil {
		slog.Error("list failed, abandoning, error", "error", err, "groupLines", groupLines)
		return
	}
	slog.Debug("abandoming error [%v] [%v]", "error", err, "groupLines", groupLines)

	for _, l := range groupLines {
		slog.Debug("lines list groups", "lines", l)
		parts := strings.Split(l, " ")
		if len(parts) < 3 {
			slog.Error("abandoming list groups", "parts", parts)
			continue
		} else {
			slog.Debug("doing list groups", "parts", parts)
		}
		high, errh := strconv.ParseInt(parts[1], 10, 64)
		low, errl := strconv.ParseInt(parts[2], 10, 64)
		if errh == nil && errl == nil {
			rv = append(rv, nntp.Group{
				Name:    parts[0],
				High:    high,
				Low:     low,
				Posting: parsePosting(parts[3]),
			})
		}
	}

	slog.Debug("sgroup ending list", "rv", rv)
	return
}

// Group selects a group.
func (c *Client) Group(name string) (rv nntp.Group, err error) {
	var msg string
	_, msg, err = c.Command("GROUP "+name, 211)
	if err != nil {
		return
	}
	// count first last name
	parts := strings.Split(msg, " ")
	if len(parts) != 4 {
		err = errors.New("Don't know how to parse result: " + msg)
	}
	rv.Count, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return
	}
	rv.Low, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}
	rv.High, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return
	}
	rv.Name = parts[3]

	return
}

// Article grabs an article
func (c *Client) Article(specifier string) (int64, string, io.Reader, error) {
	err := c.conn.PrintfLine("ARTICLE %s", specifier)
	if err != nil {
		return 0, "", nil, err
	}
	return c.articleish(220)
}

// Head gets the headers for an article
func (c *Client) Head(specifier string) (int64, string, io.Reader, error) {
	err := c.conn.PrintfLine("HEAD %s", specifier)
	if err != nil {
		return 0, "", nil, err
	}
	return c.articleish(221)
}

// Body gets the body of an article
func (c *Client) Body(specifier string) (int64, string, io.Reader, error) {
	err := c.conn.PrintfLine("BODY %s", specifier)
	if err != nil {
		return 0, "", nil, err
	}
	return c.articleish(222)
}

func (c *Client) articleish(expected int) (int64, string, io.Reader, error) {
	_, msg, err := c.conn.ReadCodeLine(expected)
	if err != nil {
		return 0, "", nil, err
	}
	parts := strings.SplitN(msg, " ", 2)
	n, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", nil, err
	}
	return n, parts[1], c.conn.DotReader(), nil
}

// Post a new article
//
// The reader should contain the entire article, headers and body in
// RFC822ish format.
func (c *Client) Post(r io.Reader) error {
	err := c.conn.PrintfLine("POST")
	if err != nil {
		return err
	}
	_, _, err = c.conn.ReadCodeLine(340)
	if err != nil {
		return err
	}
	w := c.conn.DotWriter()
	_, err = io.Copy(w, r)
	if err != nil {
		// This seems really bad
		return err
	}
	w.Close()
	_, _, err = c.conn.ReadCodeLine(240)
	return err
}

// Command sends a low-level command and get a response.
//
// This will return an error if the code doesn't match the expectCode
// prefix.  For example, if you specify "200", the response code MUST
// be 200 or you'll get an error.  If you specify "2", any code from
// 200 (inclusive) to 300 (exclusive) will be success.  An expectCode
// of -1 disables this behavior.
func (c *Client) Command(cmd string, expectCode int) (int, string, error) {
	err := c.conn.PrintfLine(cmd)
	if err != nil {
		return 0, "", err
	}
	return c.conn.ReadCodeLine(expectCode)
}

// asLines issues a command and returns the response's data block as lines.
func (c *Client) asLines(cmd string, expectCode int) ([]string, error) {
	_, _, err := c.Command(cmd, expectCode)
	if err != nil {
		return nil, err
	}
	return c.conn.ReadDotLines()
}

// Capabilities retrieves a list of supported capabilities.
//
// See https://datatracker.ietf.org/doc/html/rfc3977#section-5.2.2
func (c *Client) Capabilities() ([]string, error) {
	caps, err := c.asLines("CAPABILITIES", 101)
	if err != nil {
		return nil, err
	}
	for i, line := range caps {
		caps[i] = strings.ToUpper(line)
	}
	c.capabilities = caps
	return caps, nil
}

// GetCapability returns a complete capability line.
//
// "Each capability line consists of one or more tokens, which MUST be
// separated by one or more space or TAB characters."
//
// From https://datatracker.ietf.org/doc/html/rfc3977#section-3.3.1
func (c *Client) GetCapability(capability string) string {
	capability = strings.ToUpper(capability)
	for _, capa := range c.capabilities {
		i := strings.IndexAny(capa, "\t ")
		if i != -1 && capa[:i] == capability {
			return capa
		}
		if capa == capability {
			return capa
		}
	}
	return ""
}

// HasCapabilityArgument indicates whether a capability arg is supported.
//
// Here, "argument" means any token after the label in a capabilities response
// line. Some, like "ACTIVE" in "LIST ACTIVE", are not command arguments but
// rather "keyword" components of compound commands called "variants."
//
// See https://datatracker.ietf.org/doc/html/rfc3977#section-9.5
func (c *Client) HasCapabilityArgument(
	capability, argument string,
) (bool, error) {
	if c.capabilities == nil {
		return false, errors.New("Capabilities unpopulated")
	}
	capLine := c.GetCapability(capability)
	if capLine == "" {
		return false, errors.New("No such capability")
	}
	argument = strings.ToUpper(argument)
	for _, capArg := range strings.Fields(capLine)[1:] {
		if capArg == argument {
			return true, nil
		}
	}
	return false, nil
}

// ListOverviewFmt performs a LIST OVERVIEW.FMT query.
//
// According to the spec, the presence of an "OVER" line in the capabilities
// response means this LIST variant is supported, so there's no reason to
// check for it among the keywords in the "LIST" line, strictly speaking.
//
// See https://datatracker.ietf.org/doc/html/rfc3977#section-3.3.2
func (c *Client) ListOverviewFmt() ([]string, error) {
	fields, err := c.asLines("LIST OVERVIEW.FMT", 215)
	if err != nil {
		return nil, err
	}
	return fields, nil
}

/*
"0" or article number (see below)
Subject header content
From header content
Date header content
Message-ID header content
References header content
:bytes metadata item
:lines metadata item
*/
type OverItem struct {
	Number        string
	From          string
	Subject       string
	Date          string
	MessageId     string
	References    string
	bytesMetadata string
	linesMetadata string
}

// Over returns a list of raw overview lines with tab-separated fields.
func (c *Client) Over(args ...int) ([]OverItem, error) {
	cmd := ""
	switch len(args) {
	case 0:
		cmd = "OVER"
	case 1:
		cmd = fmt.Sprintf("OVER %d", args[0])
	case 2:
		cmd = fmt.Sprintf("OVER %d-%d", args[0], args[1])
	default:
		return nil, errors.New("Invalid arguments, either 1 or 2 numbers for an item, for a range")
	}

	// fmt.Sprintf("%d-%d", a.Low, a.High)
	lines, err := c.asLines(cmd, 224)
	if err != nil {
		return nil, err
	}
	ret := []OverItem{}
	for _, item := range lines {
		splitItem := strings.Split(item, "\t")
		slog.Debug("Split Items:", "items", splitItem)
		if len(splitItem) < 5 {
			continue
		}
		ret = append(ret, OverItem{
			Number:        splitItem[0],
			Subject:       splitItem[1],
			From:          splitItem[2],
			Date:          splitItem[3],
			MessageId:     splitItem[4],
			References:    splitItem[5],
			bytesMetadata: splitItem[6],
			linesMetadata: splitItem[7],
		})
	}
	return ret, nil
}

func (c *Client) HasTLS() bool {
	return c.tls
}

// StartTLS sends the STARTTLS command and refreshes capabilities.
//
// See https://datatracker.ietf.org/doc/html/rfc4642 and net/smtp.go, from
// which this was adapted, and maybe NNTP.startls in Python's nntplib also.
func (c *Client) StartTLS(config *tls.Config) error {
	if c.tls {
		return errors.New("TLS already active")
	}
	_, _, err := c.Command("STARTTLS", 382)
	if err != nil {
		return err
	}
	c.netconn = tls.Client(c.netconn, config)
	c.conn = textproto.NewConn(c.netconn)
	c.tls = true
	_, err = c.Capabilities()
	if err != nil {
		return err
	}
	return nil
}

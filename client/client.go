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
	"errors"
	"io"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/kothawoc/go-nntp"
)

// Client is an NNTP client.
type Client struct {
	conn   *textproto.Conn
	Banner string
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
	_, _, err = c.Command("LIST "+sub, 215)
	if err != nil {
		return
	}
	var groupLines []string
	groupLines, err = c.conn.ReadDotLines()
	if err != nil {
		return
	}
	rv = make([]nntp.Group, 0, len(groupLines))
	for _, l := range groupLines {
		parts := strings.Split(l, " ")
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

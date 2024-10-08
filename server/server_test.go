/*
 * The MIT License (MIT)
 *
 * Copyright (c) 2015 Simon Schmidt
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

package nntpserver

import (
	"math"
	"testing"
)

type rangeExpectation struct {
	input string
	low   int64
	high  int64
}

var rangeExpectations = []rangeExpectation{
	{"", 0, math.MaxInt64},
	{"73-", 73, math.MaxInt64},
	{"73-1845", 73, 1845},
}

func TestRangeEmpty(t *testing.T) {
	for _, e := range rangeExpectations {
		l, h := parseRange(e.input)
		if l != e.low {
			t.Fatalf("Error parsing %q, got low=%v, wanted %v",
				e.input, l, e.low)
		}
		if h != e.high {
			t.Fatalf("Error parsing %q, got high=%v, wanted %v",
				e.input, h, e.high)
		}
	}
}

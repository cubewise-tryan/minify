package minify

import (
	"bytes"
	"io"
	"strings"

	"code.google.com/p/go.net/html"
)

var specialTagMap = map[string]bool{
	"style":    true,
	"script":   true,
	"pre":      true,
	"code":     true,
	"textarea": true,
	"noscript": true,
}

var inlineTagMap = map[string]bool{
	"b":       true,
	"big":     true,
	"i":       true,
	"small":   true,
	"tt":      true,
	"abbr":    true,
	"acronym": true,
	"cite":    true,
	"dfn":     true,
	"em":      true,
	"kbd":     true,
	"strong":  true,
	"samp":    true,
	"var":     true,
	"a":       true,
	"bdo":     true,
	"img":     true,
	"map":     true,
	"object":  true,
	"q":       true,
	"span":    true,
	"sub":     true,
	"sup":     true,
	"button":  true,
	"input":   true,
	"label":   true,
	"select":  true,
}

var booleanAttrMap = map[string]bool{
	"allowfullscreen": true,
	"async":           true,
	"autofocus":       true,
	"autoplay":        true,
	"checked":         true,
	"compact":         true,
	"controls":        true,
	"declare":         true,
	"default":         true,
	"defaultChecked":  true,
	"defaultMuted":    true,
	"defaultSelected": true,
	"defer":           true,
	"disabled":        true,
	"draggable":       true,
	"enabled":         true,
	"formnovalidate":  true,
	"hidden":          true,
	"undeterminate":   true,
	"inert":           true,
	"ismap":           true,
	"itemscope":       true,
	"multiple":        true,
	"muted":           true,
	"nohref":          true,
	"noresize":        true,
	"noshade":         true,
	"novalidate":      true,
	"nowrap":          true,
	"open":            true,
	"pauseonexit":     true,
	"readonly":        true,
	"required":        true,
	"reversed":        true,
	"scoped":          true,
	"seamless":        true,
	"selected":        true,
	"sortable":        true,
	"spellcheck":      true,
	"translate":       true,
	"truespeed":       true,
	"typemustmatch":   true,
	"visible":         true,
}

var caseInsensitiveAttrMap = map[string]bool{
	"accept-charset": true,
	"accept":         true,
	"align":          true,
	"alink":          true,
	"axis":           true,
	"bgcolor":        true,
	"charset":        true,
	"clear":          true,
	"codetype":       true,
	"color":          true,
	"dir":            true,
	"enctype":        true,
	"face":           true,
	"frame":          true,
	"hreflang":       true,
	"http-equiv":     true,
	"lang":           true,
	"language":       true,
	"link":           true,
	"media":          true,
	"method":         true,
	"rel":            true,
	"rev":            true,
	"rules":          true,
	"scope":          true,
	"scrolling":      true,
	"shape":          true,
	"target":         true,
	"text":           true,
	"type":           true,
	"valign":         true,
	"valuetype":      true,
	"vlink":          true,
}

var urlAttrMap = map[string]bool{
	"href":       true,
	"src":        true,
	"cite":       true,
	"action":     true,
	"profile":    true,
	"xmlns":      true,
	"formaction": true,
	"poster":     true,
	"manifest":   true,
	"icon":       true,
	"codebase":   true,
	"longdesc":   true,
	"background": true,
	"classid":    true,
	"usemap":     true,
	"data":       true,
}

////////////////////////////////////////////////////////////////

// replaceMultipleWhitespace replaces any series of whitespace characters by a single space
func replaceMultipleWhitespace(s []byte) []byte {
	j := 0
	t := make([]byte, len(s))
	previousSpace := false
	for _, x := range s {
		if x == ' ' || x == '\n' || x == '\r' || x == '\t' || x == '\f' {
			if !previousSpace {
				previousSpace = true
				t[j] = ' '
				j++
			}
		} else {
			previousSpace = false
			t[j] = x
			j++
		}
	}
	return t[:j]
}

// isValidUnquotedAttr returns true when the bytes can be unquoted as an HTML attribute
func isValidUnquotedAttr(s string) bool {
	for _, x := range s {
		if x == ' ' || x == '/' || x == '"' || x == '\'' || x == '`' || x >= '<' && x <= '>' || x >= '\n' && x <= '\r' {
			return false
		}
	}
	return true
}

// getAttr gets an attribute's value from a token
func getAttrVal(token html.Token, k string) string {
	for _, attr := range token.Attr {
		if attr.Key == k {
			return attr.Val
		}
	}
	return ""
}

////////////////////////////////////////////////////////////////

type token struct {
	tt    html.TokenType
	token html.Token
	text  []byte
}

type tokenFeed struct {
	z   *html.Tokenizer
	buf []*token
}

func newTokenFeed(z *html.Tokenizer) *tokenFeed {
	return &tokenFeed{z: z}
}

func (tf *tokenFeed) shift() (html.TokenType, html.Token, []byte) {
	if len(tf.buf) > 0 {
		tf.buf = tf.buf[1:]
	}
	tf.peek(0)
	return tf.buf[0].tt, tf.buf[0].token, tf.buf[0].text
}

func (tf *tokenFeed) peek(pos int) (html.TokenType, html.Token, []byte) {
	for i := len(tf.buf); i <= pos; i++ {
		t := &token{tf.z.Next(), tf.z.Token(), []byte{}}
		if t.tt == html.TextToken {
			t.text = replaceMultipleWhitespace([]byte(t.token.Data))
		}
		tf.buf = append(tf.buf, t)
	}
	return tf.buf[pos].tt, tf.buf[pos].token, tf.buf[pos].text
}

////////////////////////////////////////////////////////////////

// HTML minifies HTML5 files, it reads from r and writes to w.
// Removes unnecessary whitespace, tags, attributes, quotes and comments and typically saves 10% in size.
func (m Minifier) HTML(w io.Writer, r io.Reader) error {
	var prevText []byte         // write prevText token until next token is received, allows to look forward one token before writing away
	var specialTag []html.Token // stack array of special tags it is in
	var prevTagToken html.Token
	precededBySpace := true // on true the next prevText token must no start with a space
	defaultScriptType := "text/javascript"
	defaultStyleType := "text/css"

	z := html.NewTokenizer(r)
	tf := newTokenFeed(z)
	for {
		tt, token, text := tf.shift()
		switch tt {
		case html.ErrorToken:
			if z.Err() == io.EOF {
				if _, err := w.Write(prevText); err != nil {
					return err
				}
				return nil
			}
			return z.Err()
		case html.DoctypeToken:
			if _, err := w.Write(bytes.TrimSpace(prevText)); err != nil {
				return err
			}
			prevText = nil

			if _, err := w.Write([]byte("<!doctype html>")); err != nil {
				return err
			}
		case html.CommentToken:
			if _, err := w.Write(prevText); err != nil {
				return err
			}
			prevText = nil

			comment := token.Data
			// TODO: ensure that nested comments are handled properly (tokenizer doesn't handle this!)
			if strings.HasPrefix(comment, "[if") {
				prevText = []byte("<!--" + comment + "-->")
			} else if strings.HasSuffix(comment, "--") {
				// only occurs when mixed up with conditional comments
				prevText = []byte("<!" + comment + ">")
			}
		case html.TextToken:
			if _, err := w.Write(prevText); err != nil {
				return err
			}
			prevText = []byte(token.Data)

			// CSS and JS minifiers for inline code
			if len(specialTag) > 0 {
				tag := specialTag[len(specialTag)-1].Data
				if tag == "style" || tag == "script" {
					mime := getAttrVal(specialTag[len(specialTag)-1], "type")
					if mime == "" {
						// default mime types
						if tag == "script" {
							mime = defaultScriptType
						} else {
							mime = defaultStyleType
						}
					}

					if err := m.Minify(mime, w, bytes.NewBuffer(prevText)); err != nil {
						if err == ErrNotExist {
							// no minifier, write the original
							if _, err := w.Write(prevText); err != nil {
								return err
							}
						} else {
							return err
						}
					}
				} else if tag == "noscript" {
					if err := m.HTML(w, bytes.NewBuffer(prevText)); err != nil {
						return err
					}
				} else if _, err := w.Write(prevText); err != nil {
					return err
				}
				prevText = nil
				break
			}

			// whitespace removal; if after an inline element, trim left if precededBySpace
			prevText = text
			if inlineTagMap[prevTagToken.Data] {
				if precededBySpace && len(prevText) > 0 && prevText[0] == ' ' {
					prevText = prevText[1:]
				}
				precededBySpace = len(prevText) > 0 && prevText[len(prevText)-1] == ' '
			} else if len(prevText) > 0 && prevText[0] == ' ' {
				prevText = prevText[1:]
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			prevTagToken = token

			if specialTagMap[token.Data] {
				if tt == html.StartTagToken {
					specialTag = append(specialTag, token)
				} else if tt == html.EndTagToken && len(specialTag) > 0 && specialTag[len(specialTag)-1].Data == token.Data {
					specialTag = specialTag[:len(specialTag)-1]
				}
			}

			// whitespace removal; if we encounter a block or a (closing) inline element, trim the right
			if !inlineTagMap[token.Data] || (tt == html.EndTagToken && len(prevText) > 0 && prevText[len(prevText)-1] == ' ') {
				precededBySpace = true
				// do not remove when next token is text and doesn't start with a space
				if len(prevText) > 0 {
					trim := false
					i := 0
					for {
						nextTt, nextToken, nextText := tf.peek(i)
						// remove if the tag is not an inline tag (but a block tag)
						if nextTt == html.ErrorToken || ((nextTt == html.StartTagToken || nextTt == html.EndTagToken || nextTt == html.SelfClosingTagToken) && !inlineTagMap[nextToken.Data]) {
							trim = true
							break
						} else if nextTt == html.TextToken {
							// remove if the text token starts with a whitespace
							trim = len(nextText) > 0 && nextText[0] == ' '
							break
						}
						i++
					}
					if trim {
						prevText = bytes.TrimRight(prevText, " ")
						precededBySpace = false
					}
				}
			}
			if _, err := w.Write(prevText); err != nil {
				return err
			}
			prevText = nil

			if len(token.Attr) == 0 && (token.Data == "body" || token.Data == "head" || token.Data == "html" ||
				tt == html.EndTagToken && (token.Data == "colgroup" || token.Data == "dd" || token.Data == "dt" ||
					token.Data == "option" || token.Data == "td" || token.Data == "tfoot" ||
					token.Data == "th" || token.Data == "thead" || token.Data == "tbody" || token.Data == "tr")) {
				break
			} else if tt == html.EndTagToken && (token.Data == "p" || token.Data == "li") {
				remove := false
				i := 1
				for {
					nextTt, nextToken, nextText := tf.peek(i)
					// continue if text token is empty or whitespace
					if nextTt != html.TextToken || (len(nextText) > 0 && string(nextText) != " ") {
						// remove only when encountering EOF, end tag (from parent) or a start tag of the same tag
						remove = (nextTt == html.ErrorToken || nextTt == html.EndTagToken || (nextTt == html.StartTagToken && nextToken.Data == token.Data))
						break
					}
					i++
				}
				if remove {
					break
				}
			}

			if token.Data == "script" || token.Data == "style" {
				if nextTt, _, _ := tf.peek(1); nextTt == html.EndTagToken {
					tf.shift()
					break
				}
			}

			if _, err := w.Write([]byte("<")); err != nil {
				return err
			}
			if tt == html.EndTagToken {
				if _, err := w.Write([]byte("/")); err != nil {
					return err
				}
			}
			if _, err := w.Write([]byte(token.Data)); err != nil {
				return err
			}

			if token.Data == "meta" && getAttrVal(token, "http-equiv") == "content-type" &&
				strings.ToLower(getAttrVal(token, "content")) == "text/html; charset=utf-8" {
				if _, err := w.Write([]byte(" charset=utf-8>")); err != nil {
					return err
				}
				break
			}

			// output attributes
			for _, attr := range token.Attr {
				val := strings.TrimSpace(attr.Val)
				if caseInsensitiveAttrMap[attr.Key] {
					val = strings.ToLower(val)
				}

				// default attribute values can be ommited
				if attr.Key == "clear" && val == "none" ||
					attr.Key == "colspan" && val == "1" ||
					attr.Key == "enctype" && val == "application/x-www-form-urlencoded" ||
					attr.Key == "frameborder" && val == "1" ||
					attr.Key == "method" && val == "get" ||
					attr.Key == "rowspan" && val == "1" ||
					attr.Key == "scrolling" && val == "auto" ||
					attr.Key == "shape" && val == "rect" ||
					attr.Key == "span" && val == "1" ||
					attr.Key == "valuetype" && val == "data" ||
					attr.Key == "language" && token.Data == "script" && val == "javascript" ||
					attr.Key == "type" && (token.Data == "script" && val == "text/javascript" ||
						token.Data == "style" && val == "text/css" ||
						token.Data == "link" && val == "text/css" ||
						token.Data == "input" && val == "text" ||
						token.Data == "button" && val == "submit") {
					continue
				}
				if _, err := w.Write([]byte(" " + attr.Key)); err != nil {
					return err
				}

				isBoolean := booleanAttrMap[attr.Key]
				if len(val) == 0 && !isBoolean {
					continue
				}

				// booleans have no value
				if !isBoolean {
					var err error
					if _, err := w.Write([]byte("=")); err != nil {
						return err
					}

					// CSS and JS minifiers for attribute inline code
					if attr.Key == "style" {
						val, err = m.MinifyString(defaultStyleType, val)
						if err != nil && err != ErrNotExist {
							return err
						}
					} else if len(attr.Key) > 2 && attr.Key[:2] == "on" {
						if len(val) >= 11 && strings.ToLower(val[:11]) == "javascript:" {
							val = val[11:]
						}
						val, err = m.MinifyString(defaultScriptType, val)
						if err != nil && err != ErrNotExist {
							return err
						}
					} else if urlAttrMap[attr.Key] {
						if len(val) >= 5 && strings.ToLower(val[:5]) == "http:" {
							val = val[5:]
						}
					} else if token.Data == "meta" && attr.Key == "content" {
						httpEquiv := getAttrVal(token, "http-equiv")
						if httpEquiv == "content-type" {
							val = strings.Replace(val, ", ", ",", -1)
						} else if httpEquiv == "content-style-type" {
							defaultStyleType = val
						} else if httpEquiv == "content-script-type" {
							defaultScriptType = val
						}

						name := strings.ToLower(getAttrVal(token, "name"))
						if name == "keywords" {
							val = strings.Replace(val, ", ", ",", -1)
						} else if name == "viewport" {
							val = strings.Replace(val, " ", "", -1)
						}
					}

					// no quote if possible, else prefer single or double depending on which occurs more often in value
					if isValidUnquotedAttr(val) {
						if _, err := w.Write([]byte(val)); err != nil {
							return err
						}
					} else if strings.Count(val, "\"") > strings.Count(val, "'") {
						if _, err := w.Write([]byte("'" + strings.Replace(val, "'", "&#39;", -1) + "'")); err != nil {
							return err
						}
					} else {
						if _, err := w.Write([]byte("\"" + strings.Replace(val, "\"", "&quot;", -1) + "\"")); err != nil {
							return err
						}
					}
				}
			}
			if _, err := w.Write([]byte(">")); err != nil {
				return err
			}
		}
	}
}

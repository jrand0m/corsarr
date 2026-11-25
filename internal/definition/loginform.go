package definition

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Pick reads one value from a page: an attribute of the first element
// matching Selector, or its text when Attr is empty.
type Pick struct {
	Selector string `yaml:"selector" json:"selector"`
	Attr     string `yaml:"attr" json:"attr"`
}

// LandingForm is what a landing page says about logging in: where the form
// posts to (as written on the page, "" when it posts to the page itself) and
// the values a browser would send before the owner types anything.
type LandingForm struct {
	Action string
	Fields url.Values
}

// skippedInputs are input types a browser does not submit unless clicked or chosen.
var skippedInputs = map[string]bool{"submit": true, "button": true, "image": true, "reset": true, "file": true}

// FormOn reads the login form (Form, default the first form) on a landing
// page: hidden inputs such as CSRF tokens, prefilled and checked inputs,
// selected options and text areas, plus the Tokens picked from elsewhere
// on the page.
func (l Login) FormOn(body []byte) (LandingForm, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return LandingForm{}, fmt.Errorf("login page: %w", err)
	}
	selector := firstOf(l.Form, "form")
	sel, err := ParseSelector(selector)
	if err != nil {
		return LandingForm{}, err
	}
	form := sel.First(doc)
	if form == nil {
		return LandingForm{}, fmt.Errorf("login page has no form matching %q", selector)
	}
	out := LandingForm{Action: strings.TrimSpace(attr(form, "action")), Fields: url.Values{}}
	walk(form, func(n *html.Node) { addControl(out.Fields, n) })
	for name, p := range l.Tokens {
		if v := (Check{Selector: p.Selector}).Attr(body, p.Attr); v != "" {
			out.Fields.Set(name, v) // a token the page lacks today is simply not sent
		}
	}
	return out, nil
}

// addControl adds what one form control would submit.
func addControl(f url.Values, n *html.Node) {
	name := attr(n, "name")
	if name == "" {
		return
	}
	switch n.Data {
	case "input":
		if v, ok := inputValue(n); ok {
			f.Add(name, v)
		}
	case "select":
		if v, ok := selectValue(n); ok {
			f.Add(name, v)
		}
	case "textarea":
		f.Add(name, textOf(n))
	}
}

// inputValue is an input's submitted value; ok is false for buttons and
// unchecked boxes.
func inputValue(n *html.Node) (string, bool) {
	typ := strings.ToLower(attr(n, "type"))
	if skippedInputs[typ] {
		return "", false
	}
	v, hasValue := attrOK(n, "value")
	if typ != "checkbox" && typ != "radio" {
		return v, true
	}
	if _, checked := attrOK(n, "checked"); !checked {
		return "", false
	}
	if !hasValue {
		v = "on"
	}
	return v, true
}

// selectValue is the selected option's value, else the first option's.
func selectValue(n *html.Node) (string, bool) {
	var first, chosen *html.Node
	walk(n, func(o *html.Node) {
		if o.Data != "option" {
			return
		}
		if first == nil {
			first = o
		}
		if _, sel := attrOK(o, "selected"); sel && chosen == nil {
			chosen = o
		}
	})
	if chosen == nil {
		chosen = first
	}
	if chosen == nil {
		return "", false
	}
	if v, ok := attrOK(chosen, "value"); ok {
		return v, true
	}
	return Text(chosen), true
}

// textOf is an element's raw text (a text area keeps its spacing).
func textOf(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}

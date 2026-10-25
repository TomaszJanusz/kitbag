// Package doccheck holds offline, test-only documentation quality checks.
package doccheck

import (
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/russross/blackfriday/v2"
)

type document struct {
	anchors map[string]bool
	links   []string
}

func TestDocumentationLinks(t *testing.T) {
	t.Parallel()
	root := os.DirFS("../..")
	files := []string{"README.md", "CONTRIBUTING.md", "AGENTS.md", "SECURITY.md"}
	err := fs.WalkDir(root, "docs", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && path.Ext(name) == ".md" {
			files = append(files, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		data, readErr := fs.ReadFile(root, name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, destination := range parseDocument(data).links {
			if linkErr := checkLink(root, name, destination); linkErr != nil {
				t.Errorf("%s: link %q: %v", name, destination, linkErr)
			}
		}
	}
}

func parseDocument(data []byte) document {
	doc := document{anchors: make(map[string]bool)}
	used := make(map[string]bool)
	// Parse Markdown rather than scanning raw text: code samples are not links,
	// and reference links, escaped destinations and nested formatting are syntax.
	tree := blackfriday.New(blackfriday.WithExtensions(
		blackfriday.CommonExtensions &^ blackfriday.HeadingIDs,
	)).Parse(data)
	tree.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		if !entering {
			return blackfriday.GoToNext
		}
		switch node.Type {
		case blackfriday.Link, blackfriday.Image:
			doc.links = append(doc.links, string(node.Destination))
		case blackfriday.Heading:
			base := headingSlug(headingText(node))
			slug := base
			for suffix := 1; used[slug]; suffix++ {
				slug = fmt.Sprintf("%s-%d", base, suffix)
			}
			used[slug] = true
			doc.anchors[slug] = true
		case blackfriday.HTMLBlock, blackfriday.HTMLSpan:
			addHTMLAnchors(doc.anchors, string(node.Literal))
		case blackfriday.Document, blackfriday.BlockQuote, blackfriday.List, blackfriday.Item,
			blackfriday.Paragraph, blackfriday.HorizontalRule, blackfriday.Emph, blackfriday.Strong,
			blackfriday.Del, blackfriday.Text, blackfriday.CodeBlock, blackfriday.Softbreak,
			blackfriday.Hardbreak, blackfriday.Code, blackfriday.Table, blackfriday.TableCell,
			blackfriday.TableHead, blackfriday.TableBody, blackfriday.TableRow:
			// Other nodes do not define links or anchors.
		}
		return blackfriday.GoToNext
	})
	return doc
}

func headingText(heading *blackfriday.Node) string {
	var text strings.Builder
	heading.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		if entering && (node.Type == blackfriday.Text || node.Type == blackfriday.Code) {
			if _, err := text.Write(node.Literal); err != nil {
				panic(err) // strings.Builder.Write always succeeds.
			}
		}
		return blackfriday.GoToNext
	})
	return html.UnescapeString(text.String())
}

func headingSlug(text string) string {
	return strings.Map(func(char rune) rune {
		switch {
		case char == ' ':
			return '-'
		case char == '-' || char == '_' || unicode.IsLetter(char) || unicode.IsNumber(char) || unicode.IsMark(char):
			return unicode.ToLower(char)
		default:
			return -1
		}
	}, text)
}

func addHTMLAnchors(anchors map[string]bool, text string) {
	tags := regexp.MustCompile(`(?i)<a\s+[^>]*>`)
	ids := regexp.MustCompile(`(?i)\s(?:id|name)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	text = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(text, "")
	for _, tag := range tags.FindAllString(text, -1) {
		for _, match := range ids.FindAllStringSubmatch(tag, -1) {
			anchors[html.UnescapeString(match[1]+match[2])] = true
		}
	}
}

func checkLink(root fs.FS, source, destination string) error {
	target, err := url.Parse(destination)
	if err != nil {
		return err
	}
	// Only relative links are in scope; nothing in this check uses the network.
	if target.Scheme != "" || target.Host != "" || strings.HasPrefix(target.Path, "/") {
		return nil
	}
	filename := source
	if target.Path != "" {
		filename = path.Join(path.Dir(source), target.Path)
	}
	info, err := fs.Stat(root, filename)
	if err != nil {
		return err
	}
	if target.Fragment == "" {
		return nil
	}
	if info.IsDir() {
		filename = path.Join(filename, "README.md")
	}
	return checkAnchor(root, filename, target.Fragment)
}

func checkAnchor(root fs.FS, filename, fragment string) error {
	data, err := fs.ReadFile(root, filename)
	if err != nil {
		return err
	}
	if !parseDocument(data).anchors[fragment] {
		return fmt.Errorf("%s: missing anchor #%s", filename, fragment)
	}
	return nil
}

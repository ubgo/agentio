// Package docscheck holds the tests that keep the public markdown of this
// repository honest. It has no non-test code: run it with
//
//	go test -count=1 ./internal/docscheck/            (task docs:check)
//	go test -count=1 ./internal/docscheck/ -run TestSnippets -args -update
//	                                                 (task docs:snippets)
//
// Files checked: README.md, CHANGELOG.md, CONTRIBUTING.md and SECURITY.md at
// the repo root, every .md under .github/ and docs/, and every README.md under
// examples/. Paths under testdata/ or docsi/, symlinks and gitignored files
// are skipped. TestNoLocalPaths and TestNoAttribution also read the Go files
// under examples/ and docs/.
//
// Conventions the tests enforce:
//
//   - Snippet sync (TestSnippets). In Go source, a region sits between the
//     lines "// snippet:start NAME" and "// snippet:end NAME"; the marker
//     lines are not part of it. In markdown, the line
//     "<!-- snippet: examples/dir/file.go#NAME -->" (path relative to the
//     repo root) goes directly above a fenced code block, which must equal
//     the region after removing common leading indentation. Without #NAME
//     the block is the whole file with every marker line removed. -update
//     rewrites the blocks from source. Unmatched markers fail.
//   - Links (TestLinks). Relative links, images, reference definitions and
//     <img src>/<a href> must name an existing file inside the repo; a
//     #anchor into a markdown file must match a heading slug (GitHub rules:
//     lowercase, drop punctuation except "-" and "_", each space becomes
//     "-", duplicates get -1, -2) or an explicit <a id>/<a name>. External
//     links are not fetched. Code (fenced or inline) is ignored.
//   - One physical line per paragraph or bullet (TestNoHardWrap). A line of
//     paragraph or list text followed by another line of plain text is a
//     hard wrap. Fenced code, indented code, tables, headings, HTML blocks
//     and blockquotes are exempt.
//   - No local machine paths and no authoring attribution
//     (TestNoLocalPaths, TestNoAttribution). Naming the products the library
//     is about is fine.
//
// Each failure is reported as "file:line: reason" with the file relative to
// the repo root.
package docscheck

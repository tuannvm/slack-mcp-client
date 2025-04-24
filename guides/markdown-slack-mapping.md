Below is a comprehensive Markdown table mapping common Markdown elements to their Slack equivalents (message and post interfaces use the same underlying “mrkdwn” syntax). Unsupported elements are noted as such.  

| Feature               | Standard Markdown Syntax           | Slack Syntax / Notes                                                  | Support           |
|-----------------------|------------------------------------|------------------------------------------------------------------------|-------------------|
| Headings              | `# Heading 1` `## Heading 2`       | Not supported in messages (Block Kit headers exist but not via `#`)    | No   |
| Paragraphs            | Blank line separates paragraphs    | No explicit paragraph syntax – use a blank line or Shift+Enter          | No   |
| Line breaks           | Two spaces at end + `\n`           | Shift+Enter (literal `\n` not parsed in message UI)                    | No   |
| Bold                  | `**bold**`                         | `*bold*` (asterisks)                                                    | Yes  |
| Italic                | `*italic*` or `_italic_`           | `_italic_` (underscores only)                                           | Partial |
| Strikethrough         | `~~strike~~`                       | `~strike~`                                                              | Partial |
| Blockquote            | `> quote`                          | `> quote`                                                               | Yes  |
| Ordered list          | `1. item`                          | `1. item`                                                               | Yes  |
| Unordered list        | `- item` or `* item`               | `• item` or `- item` (bullets rendered automatically)                  | Yes  |
| Inline code           | `` `code` ``                       | `` `code` ``                                                            | Yes  |
| Fenced code block     | <pre>```code```</pre>              | ```code```                                                               | Yes  |
| Horizontal rule       | `---`                              | Not supported                                                          | No   |
| Links                 | `[text](http://example.com)`       | `<http://example.com|text>` or auto-link by pasting URL                 | Partial |
| Images                | `![alt](http://img.png)`           | Not via Markdown – upload or drag-and-drop                              | No   |
| Tables                | `| col1 \| col2 |`                | Not supported                                                          | No   |
| Definition lists      | `Term : Definition`               | Not supported                                                          | No   |
| Footnotes             | `[^1]`                             | Not supported                                                          | No   |
| Task lists            | `- [ ] item`                       | Not supported                                                          | No   |
| HTML                  | `<br>`, `<em>`                     | Not supported                                                          | No   |
| Emoji                 | `:smile:` or Unicode 😄            | `:smile:` (auto-converted) or paste Unicode                             | Yes  |
| Automatic URL linking | `<http://example.com>`             | Paste URL – auto-linked                                                 | Yes  |
| Disable auto-link     | `<http://example.com|http://...>`  | Use `<` `|` `>` syntax to control display                              | Yes  |
: https://en.markdown.net.br/tools/slack/
# Feed fixture coverage

| Fixture | Coverage |
|---|---|
| `rss-namespaces.xml` | RSS 2.0, Dublin Core, `content:encoded`, relative links, RFC 822/1123 dates, categories, and an isolated invalid item |
| `atom-namespaces.xml` | Atom namespace, alternate links, author structure, fractional RFC 3339 dates, summaries, full content, categories, and language |
| `malformed.xml` | Safe document-level XML failure |

Additional generated test inputs cover Windows-1252, unsupported encodings, duplicate GUIDs, byte/item/depth limits, and large feeds without storing large fixtures.

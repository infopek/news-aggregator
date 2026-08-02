# Source-management validation matrix

| Input | Accepted | Boundary |
|---|---:|---|
| Absent demographic | yes | Preserved as absent and disabled |
| Present, disabled demographic | yes | Value remains stored but does not rank |
| Inferred/IP location | no interface | Only explicit country/region/city fields exist |
| Demographic weight above 0.10 | no | Domain ranking validation |
| Total enabled demographic weight above 0.20 | no | Domain ranking validation |
| HTTP/HTTPS RSS, Atom, or official API | yes | Typed adapter configuration |
| Non-HTTP source URL | no | Application URL normalization |
| Duplicate normalized URL | no new row | Existing user source wins |
| Enabled scraper without approved, dated review URLs and notes | no | Domain source validation |
| Metadata-only or explicitly full-content permission | yes | Closed permission enum |
| Credential bytes in source configuration | no | Typed configuration and vault-only command |

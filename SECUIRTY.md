
Claude finished the response
https://github.com/the-protobuf-project/protorm

write a security for this project

16:39

Fetched: GitHub - the-protobuf-project/protorm: Proto-first database schema generation for any backend.
Fetched: GitHub - the-protobuf-project/protorm: Proto-first database schema generation for any backend.
Now I have a clear picture of protorm. I'll write a SECURITY.md appropriate for this project — a Go-based protoc plugin that generates database schemas from Protobuf definitions.


Created a file, read a file
Created a file, read a file
Here's what the SECURITY.md covers, tailored specifically to protorm:

Supported versions — reflects the current v0.1.x release cadence.

Reporting process — directs reporters to GitHub's private vulnerability disclosure (the right channel for a public OSS repo) with a clear timeline and what to include in a report.

Scope — this was the most important part to get right. Because protorm is a code-generation tool rather than a running service, the threat model is different:

Path traversal via crafted proto inputs writing files outside the output directory
DDL injection through the default_value and type escape-hatch options, which are written verbatim into generated SQL
DoS against the build pipeline via malicious proto inputs
Supply-chain via Go module dependencies
User-facing guidance — practical advice for downstream users:

Treat generated SQL diffs like code before running migrations
The default_value/type verbatim-injection risk is called out explicitly since it's the most likely real-world attack vector
Recommends strict=true in CI
Credential hygiene around the generated .env.example

Security
Document · MD 
lets do stable with 1.0.0

16:42

Viewed a file, edited a file, read a file
Viewed a file, edited a file, read a file
Updated — the supported versions table now reads 1.0.x (latest) ✅ and < 1.0.0 ❌, and the wording is adjusted to reflect stable release status. Everything else stays the same.


Security
Document · MD 

Want to be notified when Claude responds?
Claude Fable 5 is currently unavailable.
Learn more(opens in new tab)




Claude is AI and can make mistakes. Please double-check responses.


Security · MD
# Security Policy
 
## Supported Versions
 
The latest stable release of protorm receives security fixes. We encourage all users to stay on the most recent version.
 
| Version | Supported |
| ------- | --------- |
| 1.0.x (latest) | ✅ |
| < 1.0.0 | ❌ |
 
## Reporting a Vulnerability
 
**Please do not report security vulnerabilities through public GitHub Issues.**
 
If you believe you have found a security vulnerability in protorm, report it privately via GitHub's built-in vulnerability reporting:
 
1. Navigate to the [Security tab](https://github.com/the-protobuf-project/protorm/security) of this repository.
2. Click **"Report a vulnerability"**.
3. Fill in the details described below.
Alternatively, you may email the maintainers directly. Check the repository's contributor list for contact information.
 
### What to include
 
A useful report covers:
 
- A clear description of the vulnerability and the potential impact.
- The protorm version(s) affected.
- A minimal reproducer: the `.proto` file, `buf.gen.yaml`, and the `opt:` flags needed to trigger the issue.
- The generated output that demonstrates the problem (e.g. malformed SQL DDL, an unsafe default, or unexpected file written outside the output directory).
- Your environment: OS, Go version, `buf` version, and which target (`prisma` / `gorm` / `sql` / `csv`) is involved.
### Response timeline
 
| Milestone | Target |
| --------- | ------ |
| Acknowledgement | 3 business days |
| Initial assessment | 7 business days |
| Fix or remediation plan | 30 days for most issues; critical issues prioritised |
| Public disclosure | Coordinated with reporter after a fix is available |
 
We will keep you updated throughout the process and credit you in the release notes unless you prefer to remain anonymous.
 
## Scope
 
protorm is a **code-generation tool** — it runs at development or CI time and produces static files. The attack surface is therefore different from a running service. Areas we consider in scope include:
 
### In scope
 
- **Malicious or crafted `.proto` inputs** that cause protorm to:
  - Write files outside the declared output directory (path traversal).
  - Generate SQL or code that contains injected statements or expressions exploitable at migration or runtime (e.g. unsanitised `default_value`, `type`, or `column` option values written verbatim into DDL or Go source).
  - Consume unbounded memory or CPU (denial of service against the build pipeline).
  - Panic or crash in a way that could be triggered by a shared CI environment processing untrusted protos.
- **Supply-chain concerns**: vulnerabilities in protorm's Go dependencies that affect the plugin binary or the generated output.
- **Generated output safety**: defaults or patterns in the generated schema that predictably introduce SQL injection, privilege escalation, or data-exposure risks in downstream applications.
- **`protorm.yaml` config parsing**: path-glob injection or directory traversal via the `match`, `database`, or `schema` fields.
### Out of scope
 
- Vulnerabilities in the **databases** that protorm targets (PostgreSQL, MongoDB, Prisma, GORM). Report those to the respective upstream projects.
- Security of **applications built on top of** protorm-generated schemas. protorm gives you the schema; securing the application is the application owner's responsibility.
- Issues that require an already-compromised developer machine or build system.
- Theoretical concerns with no demonstrated impact (e.g. "SHA-1 is weak" without a concrete exploit path).
## Security Considerations for Users
 
### Treat generated output as code
 
protorm writes SQL DDL, Go source files, and Prisma schemas. Apply the same review discipline you would to hand-written code:
 
- Review generated diffs before running `prisma migrate dev` or executing DDL against a production database.
- Do not pipe generated SQL directly to a database without inspection, especially when the source protos come from an external or untrusted party.
### The `default_value` and `type` escape hatches
 
The `(protorm.v1.column).default_value` and `(protorm.v1.column).type` options are written **verbatim** into generated DDL and Prisma schemas. If you allow external contributors to set these fields, review their values carefully — a malicious `default_value` can inject arbitrary SQL that executes during schema migration.
 
### Connection URLs in generated files
 
The generated `.env.example` and datasource config files document where to set database connection URLs. Never commit real credentials. The `.gitignore` in each generated Prisma project excludes `.env`, but verify this if you customise the output layout.
 
### `strict` mode in CI
 
Consider enabling `strict=true` (or per-rule strictness) in your `buf.gen.yaml` for CI runs:
 
```yaml
opt:
  - target=sql
  - strict=ref:error,collision:error,lint:error
```
 
This surfaces schema problems as hard errors rather than silent warnings, reducing the chance that a subtle misconfiguration reaches a migration.
 
### Dependency pinning
 
protorm generates a `package.json` for the Prisma target. Pin dependency versions and run `npm audit` as part of your workflow to catch vulnerabilities in the generated Node.js project before deploying it.
 
## Acknowledgements
 
We thank everyone who takes the time to responsibly disclose security issues. Contributors who report valid vulnerabilities will be credited in the relevant release notes.
 

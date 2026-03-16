# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x     | :white_check_mark: |

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability, please report it responsibly.

### How to Report

**Do NOT create a public GitHub issue.**

Instead, please either:

1. **GitHub Security Advisories (Preferred)**
   - Go to [Security Advisories](https://github.com/DockRouter/dockrouter/security/advisories)
   - Click "Report a vulnerability"
   - Fill in the details

2. **Email**
   - Send details to: security@dockrouter.dev
   - Include: description, steps to reproduce, potential impact

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Affected versions (if known)
- Potential impact
- Any suggested fixes (optional)

### Response Timeline

- **Initial Response**: Within 48 hours
- **Triage**: Within 7 days
- **Fix Development**: Depends on severity
- **Disclosure**: After fix is released

### Disclosure Policy

- We follow [responsible disclosure](https://en.wikipedia.org/wiki/Responsible_disclosure)
- We ask that you give us reasonable time to fix the issue before public disclosure
- We will credit you in the security advisory (unless you prefer to remain anonymous)

## Security Features

DockRouter is designed with security in mind:

- **Zero dependencies** - No supply chain attack surface
- **Minimal attack surface** - Single binary, no plugins
- **Constant-time auth** - bcrypt comparison resistant to timing attacks
- **Scratch-based Docker image** - Minimal container footprint
- **No external network calls** - Except for ACME (Let's Encrypt)

## Security Best Practices

When deploying DockRouter:

1. **Protect the Docker socket** - Mount as read-only (`:ro`)
2. **Secure the admin port** - Bind to localhost or use firewall rules
3. **Use strong auth credentials** - Generate bcrypt hashes with high cost
4. **Keep updated** - Use the latest release version
5. **Review labels** - Only expose containers that need to be public

```bash
# Example: Secure admin port binding
docker run -p 9090:9090 \
  -e DR_ADMIN_ADDR=127.0.0.1:9090 \
  ...
```

Thank you for helping keep DockRouter secure!

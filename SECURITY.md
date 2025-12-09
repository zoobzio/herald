# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible for receiving such patches depends on the CVSS v3.0 Rating:

| Version | Supported          | Status |
| ------- | ------------------ | ------ |
| latest  | ✅ | Active development |
| < latest | ❌ | Security fixes only for critical issues |

## Reporting a Vulnerability

We take the security of herald seriously. If you have discovered a security vulnerability in this project, please report it responsibly.

### How to Report

**Please DO NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via one of the following methods:

1. **GitHub Security Advisories** (Preferred)
   - Go to the [Security tab](https://github.com/zoobzio/herald/security) of this repository
   - Click "Report a vulnerability"
   - Fill out the form with details about the vulnerability

2. **Email**
   - Send details to the repository maintainer through GitHub profile contact information
   - Use PGP encryption if possible for sensitive details

### What to Include

Please include the following information (as much as you can provide) to help us better understand the nature and scope of the possible issue:

- **Type of issue** (e.g., race condition, deadlock, memory leak, etc.)
- **Full paths of source file(s)** related to the manifestation of the issue
- **The location of the affected source code** (tag/branch/commit or direct URL)
- **Any special configuration required** to reproduce the issue
- **Step-by-step instructions** to reproduce the issue
- **Proof-of-concept or exploit code** (if possible)
- **Impact of the issue**, including how an attacker might exploit the issue
- **Your name and affiliation** (optional)

### What to Expect

- **Acknowledgment**: We will acknowledge receipt of your vulnerability report within 48 hours
- **Initial Assessment**: Within 7 days, we will provide an initial assessment of the report
- **Resolution Timeline**: We aim to resolve critical issues within 30 days
- **Disclosure**: We will coordinate with you on the disclosure timeline

### Preferred Languages

We prefer all communications to be in English.

## Security Best Practices

When using herald in your applications, we recommend:

1. **Keep Dependencies Updated**
   ```bash
   go get -u github.com/zoobzio/herald
   ```

2. **Resource Management**
   - Close publishers and subscribers when no longer needed
   - Close providers to release broker connections
   - Handle context cancellation properly

3. **Error Handling**
   - Hook into `herald.ErrorSignal` for operational errors
   - Implement proper error handling in capitan hooks
   - Log errors appropriately

4. **Input Validation**
   - Validate message payloads in subscribers
   - Use type assertions safely
   - Handle deserialization errors gracefully

5. **Broker Security**
   - Use TLS for broker connections
   - Configure authentication where supported
   - Follow broker-specific security guidelines

6. **Codec Security**
   - Be cautious with custom codecs that deserialize untrusted data
   - Consider payload size limits
   - Validate deserialized data

## Security Features

herald includes several built-in security features:

- **Type Safety**: Generic publishers/subscribers provide compile-time type checking
- **Error Isolation**: Codec errors emit to ErrorSignal without crashing
- **Ack/Nack Semantics**: Failed messages can be redelivered
- **Metadata Immutability**: Context metadata is copied to prevent mutation
- **Nil Guards**: Nil codec defaults to safe JSON implementation

## Automated Security Scanning

This project uses:

- **CodeQL**: GitHub's semantic code analysis for security vulnerabilities
- **golangci-lint**: Static analysis including security linters (gosec)
- **Codecov**: Coverage tracking to ensure security-critical code is tested

## Vulnerability Disclosure Policy

- Security vulnerabilities will be disclosed via GitHub Security Advisories
- We follow a 90-day disclosure timeline for non-critical issues
- Critical vulnerabilities may be disclosed sooner after patches are available
- We will credit reporters who follow responsible disclosure practices

## Credits

We thank the following individuals for responsibly disclosing security issues:

_This list is currently empty. Be the first to help improve our security!_

---

**Last Updated**: 2024-12-07

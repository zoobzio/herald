---
name: Bug report
about: Create a report to help us improve
title: '[BUG] '
labels: 'bug'
assignees: ''

---

**Describe the bug**
A clear and concise description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:
1. Create provider with '...'
2. Start publisher/subscriber '...'
3. See error

**Code Example**
```go
// Minimal code example that reproduces the issue
package main

import (
    "github.com/zoobzio/herald"
    "github.com/zoobzio/herald/pkg/kafka"
)

func main() {
    // Your code here
}
```

**Expected behavior**
A clear and concise description of what you expected to happen.

**Actual behavior**
What actually happened, including any error messages or stack traces.

**Environment:**
 - OS: [e.g. macOS, Linux, Windows]
 - Go version: [e.g. 1.23.0]
 - herald version: [e.g. v0.1.0]
 - Provider: [e.g. kafka, sqs, nats]
 - Broker version (if applicable): [e.g. Kafka 3.0]

**Additional context**
Add any other context about the problem here.

---
name: QAChecker
role: worker
tools:
  - read_file
tools_inline:
  - name: word_count
    type: cli
    description: Count words in a file
    command: "sh -c 'wc -w \"{{path}}\"'"
    parameters:
      path:
        type: string
        description: File to count
        required: true
settings:
  max_total_tokens: 50000
  max_iterations: 2
---
Validate the final output for completeness, accuracy, and format.
Run automated checks and report any issues.

# 06 — Advanced Strategies (Modular)

Same as [single/06-advanced-strategies](../../single/06-advanced-strategies/) but each strategy is a modular project.

```bash
rakitsu run examples/modular/06-advanced-strategies/hierarchical/config.yaml "Review the auth module"
rakitsu run examples/modular/06-advanced-strategies/plan-and-execute/config.yaml "Migrate users table"
rakitsu run examples/modular/06-advanced-strategies/pipeline-loop/config.yaml "Write a sorting algorithm"
rakitsu run examples/modular/06-advanced-strategies/pipeline-dag/config.yaml "Generate a REST API"
```

## Structure

```
hierarchical/        # Supervisor assigns reviewers dynamically
plan-and-execute/    # Plan first, then execute steps
pipeline-loop/       # Iterative refinement until quality passes
pipeline-dag/        # Parallel with depends_on ordering
```

Each subdirectory has its own `config.yaml`, `agents/`, and `tools/`.

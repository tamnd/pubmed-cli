---
title: "Quick start"
description: "Run your first pubmed command."
weight: 30
---

Once `pubmed` is on your `PATH`:

```bash
pubmed --help       # see the command tree
pubmed version      # build info
```

This is a fresh scaffold, so the command tree is just `version` for now. Add
your first real command in `cli/`, build on the `pubmed` library package,
and document it here.

A good first command usually fetches one thing and prints it as JSON, so the
output pipes straight into `jq` and the rest of your tools.

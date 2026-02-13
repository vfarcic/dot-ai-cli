# Shell Completion

Enable command and flag autocompletion for your shell.

## Bash

**Linux:**
```bash
dot-ai completion bash | sudo tee /etc/bash_completion.d/dot-ai > /dev/null
```

**macOS:**
```bash
dot-ai completion bash > $(brew --prefix)/etc/bash_completion.d/dot-ai
```

Then restart your shell or source the completion file:
```bash
source $(brew --prefix)/etc/bash_completion.d/dot-ai
```

## Zsh

```bash
dot-ai completion zsh > "${fpath[1]}/_dot-ai"
```

Then restart your shell or run:
```bash
compinit
```

## Fish

```bash
dot-ai completion fish > ~/.config/fish/completions/dot-ai.fish
```

Then restart your shell or run:
```bash
source ~/.config/fish/completions/dot-ai.fish
```

## What Gets Completed

Shell completion provides:

- **Commands** — All available CLI commands
- **Flags** — Global and command-specific flags
- **Enum values** — Valid values for flags like `--output` (`yaml`, `json`)
- **Help** — Press tab to see available options

## Next Steps

- **[Commands Overview](../guides/cli-commands-overview.md)** — Learn all available commands
- **[Configuration](configuration.md)** — Configure server URL and authentication

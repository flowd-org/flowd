---
title: "Shell Completions"
weight: 9
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Enable tab-completion so commands, flags and values are suggested automatically.

## Generate completion scripts

Bash:

```bash
$ flwd completion bash > ~/.local/share/flwd.bash
$ echo 'source ~/.local/share/flwd.bash' >> ~/.bashrc
$ source ~/.bashrc
```

Zsh:

```bash
$ flwd completion zsh > ~/.local/share/flwd.zsh
$ echo 'source ~/.local/share/flwd.zsh' >> ~/.zshrc
$ source ~/.zshrc
```

Fish:

```bash
$ flwd completion fish > ~/.config/fish/completions/flwd.fish
```

PowerShell:

```bash
$ flwd completion powershell > $env:USERPROFILE\flwd.ps1
# Then add the following to your profile:
# . $env:USERPROFILE\flwd.ps1
```

Once sourced, hitting `TAB` after `flwd` or after a flag will suggest valid
subcommands, job names and arguments.

## How completions stay in sync

Completions are generated from the same job catalogue and argument
specifications that the CLI and API use. When you add or update jobs, the
completion engine sees the changes automatically.

For more details about aliases and how they appear in completion results, see
[Aliases & Intelligent Completion]({{< ref "aliases-completion.md" >}}).

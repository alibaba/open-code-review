# Interactive review workspace

Run `ocr tui` from a Git repository, or select a repository explicitly:

```sh
ocr tui --repo /path/to/repository
ocr tui --from main --to feature
ocr tui --commit HEAD
```

The workspace requires an interactive terminal. Provider and model settings use
the same configuration as `ocr review`. Use `/provider` or `/model` to open the
configuration screen. Credential commands used by reviews must work without
interactive stdin.

Use `/login` to open the existing ChatGPT browser sign-in flow. When it finishes,
use `/provider` to configure ChatGPT and a model available to your account.
Once configured, `/provider chatgpt` selects it for the next review. Login keeps
the current provider, model, and review scope. If plan usage is disabled, the
workspace shows the CLI command for granting permission; `/login` does not request
that permission automatically.

## Review changes

Type a description to set the review background, then use these commands:

| Command | Action |
| --- | --- |
| `/scope` | Choose the working tree, a revision range, or a commit |
| `/preview` | Inspect selected and excluded files |
| `/review` | Start a review of the current scope |
| `/logs` | Show recent review progress |
| `/sessions` | Browse saved runs and their findings |
| `/login` | Sign in with ChatGPT |
| `/settings` | Change and save the interface language |
| `/help` | Show available commands |
| `/quit` | Close the workspace |

Use the arrow keys and Enter to open a finding or saved session. Esc returns to
the previous screen. During a review, Esc requests cancellation and waits for
the child process to save its available results. PgUp and PgDn scroll the view.

The interface supports English, Simplified Chinese, Japanese, Korean, and
Russian. This setting controls interface text independently of review output.

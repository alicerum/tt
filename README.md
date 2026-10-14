# tt

A personal task tracker in the terminal. Capture asks fast, sort them in an
Inbox, plan today against your calendar, and see your GitHub review requests.

**macOS only for now:** it reads Apple Calendar and sends macOS notifications.

## Install

You need Go and the Xcode command line tools (`xcode-select --install`).

```sh
make install   # puts tt and tt-cal in ~/.local/bin
make config    # copies the default config to ~/.config/tt/config.toml
```

Make sure `~/.local/bin` is on your `PATH`.

## Use

```sh
tt                                    # open the planner (press ? for keys)
tt add 'Reply to Ana @ana !2 ~s ^fri' # capture a task
```

In a task line, `!1`–`!3` is impact (who notices if it slips: a person, one
team, more than one team), `^fri` the due date, `~s` the size.

## Config

Edit `~/.config/tt/config.toml`. Every option is listed there with its default.

- **Calendar:** the first run asks for calendar access. Allow it.
- **GitHub reviews:** needs the `gh` CLI, logged in (`gh auth login`).
- **Notifications:** off by default. Turn them on under `[notify]`.

## License

MIT, see [LICENSE](LICENSE).

# tasks

| File | What it does |
|------|--------------|
| `main.yml` | Creates the root-only snapshot directory; if missing, creates an orphan token with only the `snapshot` policy (on this machine) and stores it on the VPS (`0600`); installs the script, service and timer; enables the timer. |

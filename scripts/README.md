# r6-dissect misc scripts

## dump_filter_by_time.sh

This bash script extracts one second of replay from a replay dump into a new text file.

```bash
r6-dissect --dump dump.txt replay.rec

./dump_filter_by_time.sh dump.txt 0:01
# "Data saved to 0_01.txt"
```

## scoreboard_row_block_report.ps1

This PowerShell script extracts the late Y11 scoreboard row blocks from a debug log generated with `R6_SCOREBOARD_DEBUG=1`.

```powershell
$env:R6_SCOREBOARD_DEBUG='1'
go run . -d test-files\match9\Match-2026-03-13_18-53-47-25248-R04.rec *>&1 |
  Out-File -Encoding utf8 match9-r04-scoreboard.log

.\scripts\scoreboard_row_block_report.ps1 `
  -LogPath .\match9-r04-scoreboard.log `
  -Offsets 62326426,62326490 `
  -Markdown
```

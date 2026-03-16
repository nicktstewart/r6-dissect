param(
    [Parameter(Mandatory = $true)]
    [string]$LogPath,
    [int[]]$Offsets,
    [switch]$Markdown
)

$raw = Get-Content -Raw -Path $LogPath
$clean = [regex]::Replace($raw, "\x1B\[[0-9;]*[A-Za-z]", "")
$compact = [regex]::Replace($clean, "\s+", "")
$pattern = 'scoreboard_row_blockfamily=(\w+)packet_offset=(\d+)row_block="shape=late_row_block;block_offsets=([0-9\-]+);cells=([^"]*?);owned_refs=([^"]*?);local_refs=([^"]*?)"'
$rows = foreach ($match in [regex]::Matches($compact, $pattern, [System.Text.RegularExpressions.RegexOptions]::Multiline)) {
    [pscustomobject]@{
        Family       = $match.Groups[1].Value
        PacketOffset = [int64]$match.Groups[2].Value
        BlockOffsets = $match.Groups[3].Value
        Cells        = $match.Groups[4].Value
        OwnedRefs    = $match.Groups[5].Value
        LocalRefs    = $match.Groups[6].Value
    }
}

if ($Offsets -and $Offsets.Count -gt 0) {
    $rows = $rows | Where-Object { $Offsets -contains [int]$_.PacketOffset }
}

$rows = $rows | Sort-Object PacketOffset

if ($Markdown) {
    foreach ($row in $rows) {
        Write-Output ('- offset `{0}` `{1}`' -f $row.PacketOffset, $row.Family)
        Write-Output ('  block: `{0}`' -f $row.BlockOffsets)
        Write-Output ('  cells: `{0}`' -f $row.Cells)
        Write-Output ('  owned refs: `{0}`' -f $row.OwnedRefs)
        Write-Output ('  local refs: `{0}`' -f $row.LocalRefs)
    }
    exit 0
}

foreach ($row in $rows) {
    Write-Output ("offset={0} family={1} block={2}" -f $row.PacketOffset, $row.Family, $row.BlockOffsets)
    Write-Output ("cells={0}" -f $row.Cells)
    Write-Output ("owned_refs={0}" -f $row.OwnedRefs)
    Write-Output ("local_refs={0}" -f $row.LocalRefs)
    Write-Output ""
}

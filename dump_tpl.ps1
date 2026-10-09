$ErrorActionPreference = 'Stop'
$raw = [System.IO.File]::ReadAllText((Join-Path $PWD 'server\game\lang.php'))

function UnescapePhp([string]$s) {
  $sb = New-Object System.Text.StringBuilder
  $i = 0
  while ($i -lt $s.Length) {
    $c = $s[$i]
    if ($c -eq [char]92 -and $i + 1 -lt $s.Length) {
      $n = $s[$i+1]
      if ($n -eq [char]92 -or $n -eq [char]39) { [void]$sb.Append($n); $i += 2; continue }
    }
    [void]$sb.Append($c); $i++
  }
  return $sb.ToString()
}

function GoQuote([string]$s) {
  $q = [string][char]34
  $r = $s.Replace([string][char]92, [string][char]92 + [string][char]92).Replace($q, [string][char]92 + $q)
  $r = $r.Replace([string][char]13, '\r').Replace([string][char]10, '\n').Replace([string][char]9, '\t')
  return $q + $r + $q
}

$map = @(
  @('report','armynumss','tplArmyNumss'),
  @('report','b_adtroopss','tplBadtroopss'),
  @('report','b_titles','tplBtitles'),
  @('report','yb_titles','tplYbtitles'),
  @('report','hero','tplHero'),
  @('report','heroname','tplHeroName'),
  @('report','b_count','tplBCount'),
  @('report','resource','tplResource'),
  @('report','troopback','tplTroopback'),
  @('report','b_sbinfo','tplBsbinfo'),
  @('report','detectreport','tplDetect'),
  @('report','get','tplGet'),
  @('report','goods','tplGoods'),
  @('report','title_end','tplTitleEnd'),
  @('report','warming','tplWarming'),
  @('report','empty','tplEmpty'),
  @('report','end','tplEnd'),
  @('report','title','tplTitle'),
  @('report','title1','tplTitle1'),
  @('battle','youend','tplYouend'),
  @('battle','invade','tplInvade'),
  @('battle','jueweiinvade','tplJueweiInv'),
  @('battle','cityinvade','tplCityInv'),
  @('battle','losthero','tplLostHero'),
  @('battle','none','tplNone'),
  @('battle','catchhero','tplCatchHero')
)

$out = New-Object System.Collections.Generic.List[string]
$out.Add('package battle')
$out.Add('')
$out.Add('// lang.go: report templates extracted byte-exact from server/game/lang.php (do not hand-edit).')
$out.Add('')
$out.Add('const (')
foreach ($e in $map) {
  $grp = $e[0]
  $key = $e[1]
  $go = $e[2]
  $pat = "\['$grp'\]\['$key'\]\s*=\s*'((?:[^'\\]|\\.)*)'"
  $ms = [regex]::Matches($raw, $pat)
  if ($ms.Count -eq 0) { Write-Error ("NOT FOUND " + $grp + " " + $key); continue }
  $m = $ms[$ms.Count-1]
  $val = UnescapePhp $m.Groups[1].Value
  $out.Add('  ' + $go + ' = ' + (GoQuote $val))
}
$out.Add(')')
$dst = Join-Path $PWD 'backend\internal\battle\lang.go'
[System.IO.File]::WriteAllText($dst, (($out -join [string][char]10) + [string][char]10), (New-Object System.Text.UTF8Encoding($false)))
Write-Output ('written ' + $out.Count)

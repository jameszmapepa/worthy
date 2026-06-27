# benchgate.awk — fail CI on a significant latency regression.
#
# Reads `benchstat -format csv OLD NEW` on stdin and exits non-zero if any
# benchmark's sec/op (time) regressed by more than THRESHOLD percent. benchstat
# only prints a percentage in the "vs base" column when the change is
# statistically significant (otherwise "~"), so this gates on real regressions,
# not run-to-run noise. Allocation/byte regressions are intentionally ignored
# here — they are gated deterministically by the strict AllocsPerRun tests.
#
# `exclude` is an optional regex of benchmark names to skip in the gate (they
# still appear in the comparison table) — for I/O-bound benchmarks whose variance
# on shared runners is too high to gate reliably (e.g. the loopback-HTTP
# BenchmarkCollect).
#
# Usage: benchstat -format csv old.txt new.txt | awk -v threshold=20 -v exclude=Collect -f benchgate.awk
BEGIN { FS = ","; metric = ""; fail = 0 }
$2 == "sec/op" && $3 == "CI" { metric = "time"; next }
$2 == "B/op" || $2 == "allocs/op" { metric = "other"; next }
metric == "time" && $1 != "" && $1 != "geomean" {
  if (exclude != "" && $1 ~ exclude) { next }
  vs = $6
  if (vs ~ /^\+[0-9]/) {
    pct = vs; gsub(/[+%]/, "", pct)
    if (pct + 0 > threshold + 0) {
      printf "REGRESSION: %s  time +%s%%  (threshold %s%%)\n", $1, pct, threshold
      fail = 1
    }
  }
}
END { if (fail) { print "\nLatency regression(s) exceeded threshold."; exit 1 } else print "No significant latency regression beyond threshold." }

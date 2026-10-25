# Statement-weighted group floors. Ratchet policy and baseline: docs/governance.md.
BEGIN {
    floor["core"] = 85
    floor["adapters"] = 88
    floor["vcs"] = 80
    floor["cmd/kitbag"] = 78
    floor["pkg/manifest"] = 88
    group_count = split("core adapters vcs cmd/kitbag pkg/manifest", groups, " ")
    prefix = "github.com/TomaszJanusz/kitbag/"
}

/^mode: / { next }

{
    # -coverpkg emits the same block from multiple test binaries. Count its
    # statements once and union hits, including hits from testscript children.
    statements[$1] = $2
    if ($3 > 0) hit[$1] = 1
}

END {
    for (block in statements) {
        package = block
        # Dynamic regex strings avoid slash-delimiter differences across awks.
        sub("/[^/]*$", "", package)
        if (index(package, prefix) != 1) {
            print "ERROR: unexpected coverage path: " package
            failed = 1
            continue
        }
        package = substr(package, length(prefix) + 1)
        group = package
        if (package ~ "^internal/(core|adapters|vcs)(/|$)") {
            split(package, parts, "/")
            group = parts[2]
        }
        if (!(group in floor)) {
            print "ERROR: unexpected coverage group: " group
            failed = 1
            continue
        }
        total[package] += statements[block]
        covered[package] += statements[block] * hit[block]
        group_total[group] += statements[block]
        group_covered[group] += statements[block] * hit[block]
    }
    printf "%-52s %9s %10s\n", "PACKAGE", "COVERAGE", "HIT/TOTAL"
    for (package in total) packages[++package_count] = package
    # POSIX awk has no asorti; a small insertion sort keeps Linux/macOS output stable.
    for (i = 2; i <= package_count; i++) {
        package = packages[i]
        j = i - 1
        while (j > 0 && packages[j] > package) {
            packages[j + 1] = packages[j]
            j--
        }
        packages[j + 1] = package
    }
    for (i = 1; i <= package_count; i++) {
        package = packages[i]
        if (total[package] > 0) {
            printf "%-52s %8.2f%% %5d/%-5d\n", package, \
                100 * covered[package] / total[package], covered[package], total[package]
        }
    }
    for (i = 1; i <= group_count; i++) {
        group = groups[i]
        if (group_total[group] <= 0) {
            print "FAIL group " group ": no executable statements in coverage profile"
            failed = 1
            continue
        }
        percent = 100 * group_covered[group] / group_total[group]
        status = percent >= floor[group] ? "PASS" : "FAIL"
        printf "%s group %-14s %6.2f%% (floor %d%%; %d/%d statements)\n", \
            status, group, percent, floor[group], group_covered[group], group_total[group]
        if (percent < floor[group]) failed = 1
    }
    exit failed
}

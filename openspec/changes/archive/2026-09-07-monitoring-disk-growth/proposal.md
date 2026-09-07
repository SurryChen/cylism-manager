## Why

The monitoring workspace currently shows node root-disk utilization but cannot identify the PVCs or container filesystems responsible for rapid growth. VictoriaMetrics already collects the necessary kubelet and cAdvisor metrics, so the platform can provide an actionable diagnostic view without adding another collector.

## What Changes

- Add a bounded monitoring API that ranks positive disk growth for node mount points, PVCs and container writable filesystems.
- Map PVC rows to Pods that currently mount each claim.
- Add a lazy-loaded Monitoring Disk tab with time-range and node filters.

## Non-Goals

- No periodic host-directory `du` scans, arbitrary shell access or automatic remediation.
- No new scrape targets, retention changes or raw PromQL access from the disk UI.

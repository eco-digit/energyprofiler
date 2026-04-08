# Energyprofiler
Baremetal energy profiler for server without RAPL, turbostat or possibility to access PDU data.

## Requirements:
For BMC data access:
- ipmitool
- stress-ng

## Usage
To build benchmark binary, run:
```
make build
```

To execute move binary onto the compute node. And execute
```
make build
```


Requirements for network benchmarks:

```
- stream for network benchmarks to generate load
```

## Flag helper
```
Usage:
   ./benchmark [flags]

Flags:
      --resource string                benchmark to run: cpu, memory, memory-bandwidth, storage, network, all (default "cpu")
      --cycles int                     how many times to repeat the benchmark (default 3)
      --load-levels string             comma-separated percentages to target (default "10,25,50,75,100")
      --stabilize duration             time to wait before measurement starts (default "30s")
      --measurement-duration duration  how long to collect power + utilization (default "60s")
      --measurement-interval duration  sample interval during measurement (default "10s")
      --cooldown duration              idle time between benchmark cycles (default "30s")
      --data-source string             power data source: bmc (default "bmc")
      --output string                  path to write the results file (default "benchmark-results.json")
      --stream-path string             path to the STREAM binary (default "/usr/local/bin/stream")
      --stream-workdir string          working directory for running STREAM
      --storage-stressor string        storage stressor type: io, hdd, ssd, iomix, aio (default "io")
      --system-power                   if set, use system-wide power readings instead of component-specific
      --verbose                        enable detailed logging
  -h, --help                           show help message

```

## Default config
If no falgs are provided the tool runs with the following default settings:
```
--resource="cpu"
--cycles=3
--load-levels="10,25,50,75,100"
--stabilize="30s"
--measurement-duration="60s"
--measurement-interval="10s"
--cooldown="30s"
--output="benchmark-results.json"
--stream-path="/usr/local/bin/stream"
--verbose=false
--system-power=false
```




## DBR 1 CPU:
This benchmark stresses CPU cores using stress-ng. Define the target utilization levels (e.g. 10%, 50%, 100%), and the profiler ramps up CPU load with multiple workers and collects power readings from the configured source (at the moment only BMC; set to component power cpu only or system-wide).

#### Example:
```
# Fast func test:
./benchmark --resource cpu --cycles 1 --load-levels 10,50 --stabilize 5s --measurement-duration 10s --verbose

# Production test:
./benchmark --resource cpu --cycles 3 --load-levels 10,20,30,40,50,60,70,80,90,100 --stabilize 30s --measurement-duration 60s --measurement-interval 10s --cooldown 30s --output production-cpu-results.json

```


## DBR 2 RAM:
### Capacity Benchmark (--resource memory).
This measures how much DRAM power increases when we allocate more memory — without moving data around. We use:

* --vm-populate --> actually touches every page so RAM is really used (no lazy allocation)
* --vm-locked --> keeps pages in DRAM (no swapping)
* --vm-hang --> after filling memory, workers go idle and just hold the allocation (capacity test, not bandwidth :)

This is a capacity-only test, not bandwidth. Good for idle DRAM draw across different memory pressure levels.

#### Example:
```
./benchmark --resource memory --load-levels 25,50,75 --cycles 1 --stabilize 5s --measurement-duration 10s
```
### Bandwidth Benchmark (--resource memory-bandwidth)
This version uses the STREAM benchmark to push memory throughput (GB/s) at different intensities, scaling threads and working-set sizes.
We normalize the measured STREAM Triad value to the theoretical peak (based on RAM type and channels) to get a % bandwidth utilization, then track power over time. STREAM reports Triad bandwidth (GB/s). We normalize it against the system’s theoretical peak (from DIMM speed × channels) to get a utilization %. At least tahts the goal needs some more fine-tuning.

#### Example:
```
./benchmark --resource memory-bandwidth --cycles 1 --stabilize 10s --measurement-duration 30s --stream-path ./stream
```

## DBR 3 Store:
This benchmark will stress storage I/O (SSD/HDD) and correlate it with power usage. Currently WIP depending on your tool implementation — but the idea is to use tools like fio to push different read/write workloadsat configurable levels.

It's NOT measuring:
- Actual MB/s throughput
- I/Os achieved
- Latency
- Real utilization percentage
---
During hardware discovery, the benchmark assigns estimated peak values:
* TheoreticalIOPS: max I/O operations per second (e.g. 500K for NVMe)
* TheoreticalBW: max bandwidth in MB/s (e.g. 550 for SATA SSD)

- NVMe values (500k IOPS, 3.5 GB/s) represent common mid-range PCIe 3.0 performance
- SATA SSD values (50k IOPS, 550 MB/s) match the known theoretical bus limit of SATA III.
- HDD estimates (~200 IOPS, 150 MB/s) are upper bounds for modern 7200 RPM drives

These are not measured, but used to:
- Normalize I/O load levels
- Scale workers and ops in the benchmark
 Attribute energy use to I/O intensity (% of peak, not absolute speed)

- This helps compare performance and power usage consistently across different disk types (HDD / SSD / NVMe), they are only a rough baseline to normalize I/O load across systems.

#### Example:
```
./benchmark \
    --resource storage \
    --storage-stressor io \
    --cycles 1 \
    --load-levels 10,20,30,40,50,60,70,80,90,100 \
    --stabilize 30s \
    --measurement-duration 60s \
    --cooldown 30s \
    --output storage-production.json
```

## DBR 4 Transfer (Network):
The network benchmark stresses data transfer (TCP/UDP) using iperf3 or a similar tool between nodes or VMs. It runs test clients against an external iperf server and ramps up throughput to measure power at different utilization levels.
This helps profile NIC-related energy use (especially in multi-10G or bonded setups).

To start the networking server for steam:


#### Example:
```
./benchmark -resource transfer --load-levels 10,20,30,40,50,60,70,80,90,100 --stabilize 30s --measurement-duration 60s --cooldown 30s --output network-power-profile.json --network-server 10.1.0.41
```


## System-wide Power test:
```
./benchmark \
    --resource cpu \
    --cycles 1 \
    --load-levels 25,50,75 \
    --stabilize 10s \
    --measurement-duration 20s \
    --system-power \
    --output system-power-results.json
```

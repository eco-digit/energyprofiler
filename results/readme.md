# Usage examples
```
# To plot run
python3 pretty_power_plot.py power-profile.json

# Force 10-unit grid ticks regardless of samples:
python3 pretty_power_plot.py cpu-power-profile.json --gridstep 10

# Plot only the compute and store profiles and save to SVG:
python3 pretty_power_plot.py power-profile.json --profiles compute,store --out out.svg
```

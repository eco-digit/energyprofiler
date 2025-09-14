#!/usr/bin/env python3
"""
pretty_power_plot.py

Usage:
    python3 pretty_power_plot.py cpu-power-profile.json
    python3 pretty_power_plot.py cpu-power-profile.json --profiles compute,store --out out.png --smooth 300 --flip

Features:
 - Reads the JSON format you provided (full "power_profile" object or a single profile).
 - Flip axes with --flip (power on x, utilization on y or vice-versa).
 - Places grid lines at exact utilization sample points when they look like the "coarse" set
   [0,25,50,75,100] (or subset). Otherwise defaults to 10-unit spacing (or use --gridstep to override).
 - Uses PCHIP smoothing when scipy is available; falls back to polynomial or linear interpolation.
 - Produces a nicer-looking plot with spacing, markers, shaded area under curves,
   legend placed outside plot, and tight layout.
"""

from pathlib import Path
import argparse
import json
import math
import sys

import numpy as np
import matplotlib.pyplot as plt

# Optional shape-preserving interpolator
try:
    from scipy.interpolate import PchipInterpolator
    SCIPY_AVAILABLE = True
except Exception:
    PchipInterpolator = None
    SCIPY_AVAILABLE = False


def load_json(path: Path):
    with open(path, 'r') as f:
        return json.load(f)


def extract_profiles(data):
    """
    Return dict: {profile_name: {'x': [...], 'y': [...], 'avg': float or None}}
    Accepts both top-level 'power_profile' dict or a single profile mapping.
    - Assumes profile keys are the independent variable (normally utilization %).
    """
    profiles = {}
    if not isinstance(data, dict):
        raise ValueError("JSON root must be an object")
    if 'power_profile' in data and isinstance(data['power_profile'], dict):
        pp = data['power_profile']
        for name, val in pp.items():
            if not isinstance(val, dict):
                continue
            profile = val.get('profile')
            avg = val.get('avg')
            if profile and isinstance(profile, dict):
                xs = [float(k) for k in profile.keys()]
                ys = [float(v) for v in profile.values()]
                pairs = sorted(zip(xs, ys))
                xs_sorted = [p[0] for p in pairs]
                ys_sorted = [p[1] for p in pairs]
                profiles[name] = {'x': xs_sorted, 'y': ys_sorted, 'avg': avg}
    else:
        # treat entire file as a single profile (keys -> utilization/power)
        xs = [float(k) for k in data.keys()]
        ys = [float(v) for v in data.values()]
        pairs = sorted(zip(xs, ys))
        xs_sorted = [p[0] for p in pairs]
        ys_sorted = [p[1] for p in pairs]
        profiles['profile'] = {'x': xs_sorted, 'y': ys_sorted, 'avg': None}
    return profiles


def smooth_curve(x, y, num=300, method='auto'):
    """
    Return (x_new, y_new) with smoothing/interpolation.
    method: 'auto' uses PCHIP if available else polynomial fit degree up to 3.
    num: number of points for returned curve.
    """
    if len(x) < 2:
        return np.array(x), np.array(y)

    x = np.array(x, dtype=float)
    y = np.array(y, dtype=float)

    # create denser x range across the span with a little padding
    xmin, xmax = x.min(), x.max()
    xpad = (xmax - xmin) * 0.01 if xmax > xmin else 0.0
    x_new = np.linspace(xmin - xpad, xmax + xpad, num)

    if method == 'auto' and SCIPY_AVAILABLE:
        try:
            interp = PchipInterpolator(x, y, extrapolate=True)
            y_new = interp(x_new)
            return x_new, y_new
        except Exception:
            # fall back
            pass

    # Fallback: polynomial fit of degree min(3, n-1)
    deg = min(3, max(1, len(x) - 1))
    try:
        coefs = np.polyfit(x, y, deg=deg)
        poly = np.poly1d(coefs)
        y_new = poly(x_new)
        return x_new, y_new
    except Exception:
        # final fallback: linear interpolation
        y_new = np.interp(x_new, x, y)
        return x_new, y_new


def is_coarse_25_set(values):
    """
    Return True if the set of numeric values is a subset of {0,25,50,75,100}
    (with small float tolerance).
    """
    allowed = {0.0, 25.0, 50.0, 75.0, 100.0}
    vals_rounded = set(round(float(v), 6) for v in values)
    return vals_rounded.issubset(allowed) and len(vals_rounded) >= 2


def nice_plot(profiles, out_path=None, curvesmooth_points=300, figsize=(10, 6),
              show_avg=True, flip_axes=False, gridstep=None):
    """
    flip_axes: if True, swap x and y before plotting (i.e., power on x, utilization on y if input
               has utilization in keys).
    gridstep: if not None, use fixed step spacing for ticks (e.g., 10). Otherwise auto-detect:
              - if sample points are subset of {0,25,50,75,100}, use those sample points for ticks
              - else default to step=10
    """
    # Nice default rc
    plt.rcParams.update({
        'figure.dpi': 120,
        'font.size': 11,
        'axes.titlesize': 14,
        'axes.titleweight': 'semibold',
        'axes.labelsize': 12,
        'legend.fontsize': 10,
        'lines.linewidth': 1.9,
        'lines.markersize': 6,
    })

    fig, ax = plt.subplots(figsize=figsize)

    color_cycle = plt.rcParams['axes.prop_cycle'].by_key().get('color', None)

    handles = []
    labels = []
    y_all = []
    all_sample_indep = set()  # collect the independent variable sample points (normally utilization)

    # First pass: collect sample independent values across profiles
    for name, p in profiles.items():
        xs = p['x']
        all_sample_indep.update(xs)

    # Decide tick set for the independent axis (which may map to x or y depending on flip)
    use_coarse = is_coarse_25_set(all_sample_indep)
    xticks = None
    yticks = None

    # If user provided gridstep, honor it
    if gridstep is not None:
        # we will create ticks from global min to max with this step
        pass  # we'll compute later based on axis limits

    # Plot each profile
    for idx, (name, p) in enumerate(profiles.items()):
        x = p['x']
        y = p['y']
        if len(x) == 0:
            continue

        # If flip_axes: swap
        if flip_axes:
            plot_x = np.array(y, dtype=float)  # power -> x
            plot_y = np.array(x, dtype=float)  # util -> y
        else:
            plot_x = np.array(x, dtype=float)
            plot_y = np.array(y, dtype=float)

        x_smooth, y_smooth = smooth_curve(plot_x, plot_y, num=curvesmooth_points, method='auto')

        color = None
        if color_cycle:
            color = color_cycle[idx % len(color_cycle)]

        # Smooth line
        line, = ax.plot(x_smooth, y_smooth, label=f"{name} (smoothed)", zorder=3, color=color)
        # Original sample points (markers)
        ax.plot(plot_x, plot_y, 'o', label=f"{name} (samples)", markersize=5.5,
                zorder=4, color=color, markeredgecolor='k', markeredgewidth=0.4)
        # Fill
        # fill toward the minimum of the plotted y (so shaded area under the curve)
        ax.fill_between(x_smooth, y_smooth, np.minimum(y_smooth, np.nanmin(y_smooth)),
                        alpha=0.06, zorder=1, color=color)

        avg = p.get('avg')
        if show_avg and avg is None:
            # compute average of dependent variable (power) irrespective of flip
            avg = float(np.mean(p['y'])) if len(p['y']) > 0 else None
        if show_avg and avg is not None:
            # If axes are flipped and we want to draw a horizontal line at avg power,
            # we must put it on the dependent axis (which is y if not flipped, x if flipped).
            if flip_axes:
                # dependent is x -> vertical line
                ax.axvline(avg, linestyle='--', linewidth=1.0, alpha=0.7, color=color)
                # annotate near top-right
                ax.text(0.99, 0.95 - 0.03*len(handles), f"{name} avg {avg:.2f} W",
                        va='top', ha='right', transform=ax.transAxes, fontsize=9, color=color)
            else:
                # dependent is y -> horizontal line
                ax.axhline(avg, linestyle='--', linewidth=1.0, alpha=0.7, color=color)
                ax.text(0.99, avg, f" {name} avg {avg:.2f} W", va='center', ha='right',
                        transform=ax.get_yaxis_transform(), fontsize=9, color=color)

        handles.append(line)
        labels.append(name)
        y_all.extend(y_smooth.tolist())

    # Axis labels and title depending on flip
    if flip_axes:
        ax.set_xlabel("Power (W)")
        ax.set_ylabel("Utilization (%)")
        ax.set_title("Power vs Utilization (flipped axes)")
    else:
        ax.set_xlabel("Utilization (%)")
        ax.set_ylabel("Power (W)")
        ax.set_title("Power Profiles")

    # Set reasonable axis limits
    if len(y_all) > 0:
        ymin, ymax = min(y_all), max(y_all)
        yrange = ymax - ymin if ymax != ymin else max(1.0, abs(ymax) * 0.1)
        ax.set_ylim(ymin - 0.08 * yrange, ymax + 0.12 * yrange)

    # Decide tick positions for the utilization axis (the "sample" axis).
    # If not flipped -> utilization is x axis, else utilization is y axis.
    indep_samples = sorted(all_sample_indep)

    if gridstep is not None:
        # compute ticks from min to max using the requested step
        minv = min(indep_samples) if indep_samples else 0.0
        maxv = max(indep_samples) if indep_samples else 0.0
        # snap start to multiple of gridstep for nicer alignment
        start = math.floor(minv / gridstep) * gridstep
        end = math.ceil(maxv / gridstep) * gridstep
        ticks = list(np.arange(start, end + gridstep/2, gridstep))
    else:
        # auto: if the sample set is a subset of {0,25,50,75,100} use exact sample points
        if is_coarse_25_set(indep_samples):
            ticks = sorted(indep_samples)
        else:
            # fallback to step=10 across the range
            minv = min(indep_samples) if indep_samples else 0.0
            maxv = max(indep_samples) if indep_samples else 0.0
            # create ticks starting at 0 if minv >= 0 else from floor(minv/10)*10
            start = 0 if minv >= 0 else math.floor(minv / 10) * 10
            end = math.ceil(maxv / 10) * 10
            ticks = list(np.arange(start, end + 0.1, 10.0))

    # apply ticks and grid on the independent axis (x or y depending on flip)
    if flip_axes:
        ax.set_yticks(ticks)
        ax.grid(which='major', axis='y', linestyle=':', linewidth=0.8, alpha=0.9)
        ax.grid(which='major', axis='x', linestyle=':', linewidth=0.8, alpha=0.9)
    else:
        ax.set_xticks(ticks)
        ax.grid(which='major', axis='x', linestyle=':', linewidth=0.8, alpha=0.9)
        ax.grid(which='major', axis='y', linestyle=':', linewidth=0.8, alpha=0.9)

    # Legend outside the plot
    ax.legend(handles=handles, labels=labels, loc='center left', bbox_to_anchor=(1.02, 0.5))
    plt.tight_layout(rect=(0, 0, 0.82, 1))

    if out_path:
        fig.savefig(out_path, bbox_inches='tight', dpi=150)
        print(f"Wrote {out_path}")

    plt.show()


def main():
    parser = argparse.ArgumentParser(description="Pretty plot power profile JSON")
    parser.add_argument("jsonfile", type=Path, help="JSON file to read")
    parser.add_argument("--profiles", type=str, default=None, help="Comma-separated profile names to plot (e.g. compute,store). Default: all in file.")
    parser.add_argument("--out", type=Path, default=Path("power-profile.png"), help="Output PNG path (also shown).")
    parser.add_argument("--smooth", type=int, default=300, help="Number of points for the smoothed curves (higher -> smoother).")
    parser.add_argument("--no-avg", action='store_true', help="Do not draw average horizontal/vertical lines.")
    parser.add_argument("--flip", action='store_true', help="Flip axes: power on x-axis, utilization on y-axis.")
    parser.add_argument("--gridstep", type=float, default=None, help="Force tick/grid step (e.g. 10). If omitted, auto-detects: use sample points if they are multiples of 25 else use step=10.")
    args = parser.parse_args()

    if not args.jsonfile.exists():
        raise SystemExit(f"File not found: {args.jsonfile}")

    raw = load_json(args.jsonfile)
    profiles = extract_profiles(raw)

    if args.profiles:
        wanted = [p.strip() for p in args.profiles.split(',') if p.strip()]
        profiles = {k: v for k, v in profiles.items() if k in wanted}

    if not profiles:
        raise SystemExit("No profiles found in file or none matched -- aborting.")

    nice_plot(profiles, out_path=str(args.out), curvesmooth_points=args.smooth,
              show_avg=not args.no_avg, flip_axes=args.flip, gridstep=args.gridstep)


if __name__ == "__main__":
    main()

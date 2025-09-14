#!/usr/bin/env python3
"""
pretty_power_plot_multi.py

Like the original pretty_power_plot.py but accepts multiple JSON files and plots
all profiles found across them. Series are named "filename:profilename" so it's
easy to distinguish sources.

Usage:
    python3 pretty_power_plot_multi.py host.json vm4.json vm8.json
    python3 pretty_power_plot_multi.py host.json vm4.json --profiles compute,profile --out out.png --flip
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
    with open(path, "r") as f:
        return json.load(f)


def extract_profiles_from_data(data):
    """
    Return dict: {profile_name: {'x': [...], 'y': [...], 'avg': float or None}}
    Accepts both top-level 'power_profile' dict or a single profile mapping.
    """
    profiles = {}
    if not isinstance(data, dict):
        return profiles
    if "power_profile" in data and isinstance(data["power_profile"], dict):
        pp = data["power_profile"]
        for name, val in pp.items():
            if not isinstance(val, dict):
                continue
            profile = val.get("profile")
            avg = val.get("avg")
            if profile and isinstance(profile, dict):
                xs = [float(k) for k in profile.keys()]
                ys = [float(v) for v in profile.values()]
                pairs = sorted(zip(xs, ys))
                xs_sorted = [p[0] for p in pairs]
                ys_sorted = [p[1] for p in pairs]
                profiles[name] = {"x": xs_sorted, "y": ys_sorted, "avg": avg}
    else:
        # treat entire file as a single profile (keys -> utilization/power)
        try:
            xs = [float(k) for k in data.keys()]
            ys = [float(v) for v in data.values()]
            pairs = sorted(zip(xs, ys))
            xs_sorted = [p[0] for p in pairs]
            ys_sorted = [p[1] for p in pairs]
            profiles["profile"] = {"x": xs_sorted, "y": ys_sorted, "avg": None}
        except Exception:
            # nothing to do
            pass
    return profiles


def gather_profiles_from_files(paths, wanted_profiles=None):
    """
    Read multiple JSON files and return a dict of combined profiles with unique keys:
    "<basename>:<profilename>" -> profile-dict
    """
    all_profiles = {}
    for p in paths:
        data = load_json(p)
        profs = extract_profiles_from_data(data)
        base = p.name
        if wanted_profiles:
            profs = {k: v for k, v in profs.items() if k in wanted_profiles}
        for name, val in profs.items():
            key = f"{base}:{name}"
            all_profiles[key] = val
    return all_profiles


def smooth_curve(x, y, num=300, method="auto"):
    """
    Return (x_new, y_new) with smoothing/interpolation.
    method: 'auto' uses PCHIP if available else polynomial fit degree up to 3.
    num: number of points for returned curve.
    """
    if len(x) < 2:
        return np.array(x), np.array(y)

    x = np.array(x, dtype=float)
    y = np.array(y, dtype=float)

    xmin, xmax = x.min(), x.max()
    xpad = (xmax - xmin) * 0.01 if xmax > xmin else 0.0
    x_new = np.linspace(xmin - xpad, xmax + xpad, num)

    if method == "auto" and SCIPY_AVAILABLE:
        try:
            interp = PchipInterpolator(x, y, extrapolate=True)
            y_new = interp(x_new)
            return x_new, y_new
        except Exception:
            pass

    deg = min(3, max(1, len(x) - 1))
    try:
        coefs = np.polyfit(x, y, deg=deg)
        poly = np.poly1d(coefs)
        y_new = poly(x_new)
        return x_new, y_new
    except Exception:
        y_new = np.interp(x_new, x, y)
        return x_new, y_new


def is_coarse_25_set(values):
    allowed = {0.0, 25.0, 50.0, 75.0, 100.0}
    vals_rounded = set(round(float(v), 6) for v in values)
    return vals_rounded.issubset(allowed) and len(vals_rounded) >= 2


def nice_plot(profiles, out_path=None, curvesmooth_points=300, figsize=(10, 6),
              show_avg=True, flip_axes=False, gridstep=None):
    """
    Plot profiles dict: {name: {'x':..., 'y':..., 'avg':...}}
    See original script for behavior and flags.
    """
    plt.rcParams.update({
        "figure.dpi": 120,
        "font.size": 11,
        "axes.titlesize": 14,
        "axes.titleweight": "semibold",
        "axes.labelsize": 12,
        "legend.fontsize": 10,
        "lines.linewidth": 1.9,
        "lines.markersize": 6,
    })

    fig, ax = plt.subplots(figsize=figsize)
    color_cycle = plt.rcParams["axes.prop_cycle"].by_key().get("color", None)

    handles = []
    labels = []
    y_all = []
    all_sample_indep = set()

    # collect independent variable sample points
    for name, p in profiles.items():
        xs = p["x"]
        all_sample_indep.update(xs)

    # prepare ticks later
    indep_samples = sorted(all_sample_indep)

    for idx, (name, p) in enumerate(profiles.items()):
        x = p["x"]
        y = p["y"]
        if len(x) == 0:
            continue

        if flip_axes:
            plot_x = np.array(y, dtype=float)
            plot_y = np.array(x, dtype=float)
        else:
            plot_x = np.array(x, dtype=float)
            plot_y = np.array(y, dtype=float)

        x_smooth, y_smooth = smooth_curve(plot_x, plot_y, num=curvesmooth_points, method="auto")

        color = None
        if color_cycle:
            color = color_cycle[idx % len(color_cycle)]

        line, = ax.plot(x_smooth, y_smooth, label=f"{name} (smoothed)", zorder=3, color=color)
        ax.plot(plot_x, plot_y, "o", label=f"{name} (samples)", markersize=5.5,
                zorder=4, color=color, markeredgecolor="k", markeredgewidth=0.4)

        ax.fill_between(x_smooth, y_smooth, np.minimum(y_smooth, np.nanmin(y_smooth)),
                        alpha=0.06, zorder=1, color=color)

        avg = p.get("avg")
        if show_avg and avg is None:
            avg = float(np.mean(p["y"])) if len(p["y"]) > 0 else None
        if show_avg and avg is not None:
            if flip_axes:
                ax.axvline(avg, linestyle="--", linewidth=1.0, alpha=0.7, color=color)
                ax.text(0.99, 0.95 - 0.03*len(handles), f"{name} avg {avg:.2f} W",
                        va="top", ha="right", transform=ax.transAxes, fontsize=9, color=color)
            else:
                ax.axhline(avg, linestyle="--", linewidth=1.0, alpha=0.7, color=color)
                ax.text(0.99, avg, f" {name} avg {avg:.2f} W", va="center", ha="right",
                        transform=ax.get_yaxis_transform(), fontsize=9, color=color)

        handles.append(line)
        labels.append(name)
        y_all.extend(y_smooth.tolist())

    if flip_axes:
        ax.set_xlabel("Power (W)")
        ax.set_ylabel("Utilization (%)")
        ax.set_title("Power vs Utilization (flipped axes)")
    else:
        ax.set_xlabel("Utilization (%)")
        ax.set_ylabel("Power (W)")
        ax.set_title("Power Profiles")

    if len(y_all) > 0:
        ymin, ymax = min(y_all), max(y_all)
        yrange = ymax - ymin if ymax != ymin else max(1.0, abs(ymax) * 0.1)
        ax.set_ylim(ymin - 0.08 * yrange, ymax + 0.12 * yrange)

    # Decide ticks for independent axis
    if gridstep is not None:
        minv = min(indep_samples) if indep_samples else 0.0
        maxv = max(indep_samples) if indep_samples else 0.0
        start = math.floor(minv / gridstep) * gridstep
        end = math.ceil(maxv / gridstep) * gridstep
        ticks = list(np.arange(start, end + gridstep/2, gridstep))
    else:
        if is_coarse_25_set(indep_samples):
            ticks = sorted(indep_samples)
        else:
            minv = min(indep_samples) if indep_samples else 0.0
            maxv = max(indep_samples) if indep_samples else 0.0
            start = 0 if minv >= 0 else math.floor(minv / 10) * 10
            end = math.ceil(maxv / 10) * 10
            ticks = list(np.arange(start, end + 0.1, 10.0))

    if flip_axes:
        ax.set_yticks(ticks)
        ax.grid(which="major", axis="y", linestyle=":", linewidth=0.8, alpha=0.9)
        ax.grid(which="major", axis="x", linestyle=":", linewidth=0.8, alpha=0.9)
    else:
        ax.set_xticks(ticks)
        ax.grid(which="major", axis="x", linestyle=":", linewidth=0.8, alpha=0.9)
        ax.grid(which="major", axis="y", linestyle=":", linewidth=0.8, alpha=0.9)

    ax.legend(handles=handles, labels=labels, loc="center left", bbox_to_anchor=(1.02, 0.5))
    plt.tight_layout(rect=(0, 0, 0.82, 1))

    if out_path:
        fig.savefig(out_path, bbox_inches="tight", dpi=150)
        print(f"Wrote {out_path}")

    plt.show()


def main():
    parser = argparse.ArgumentParser(description="Pretty plot power profile JSON (multi-file)")
    parser.add_argument("jsonfiles", type=Path, nargs="+", help="One or more JSON files to read")
    parser.add_argument("--profiles", type=str, default=None, help="Comma-separated profile names to plot (applies to each file). Default: all in files.")
    parser.add_argument("--out", type=Path, default=Path("power-profile-multi.png"), help="Output PNG path (also shown).")
    parser.add_argument("--smooth", type=int, default=300, help="Number of points for the smoothed curves (higher -> smoother).")
    parser.add_argument("--no-avg", action="store_true", help="Do not draw average horizontal/vertical lines.")
    parser.add_argument("--flip", action="store_true", help="Flip axes: power on x-axis, utilization on y-axis.")
    parser.add_argument("--gridstep", type=float, default=None, help="Force tick/grid step (e.g. 10). If omitted, auto-detects.")
    args = parser.parse_args()

    for p in args.jsonfiles:
        if not p.exists():
            raise SystemExit(f"File not found: {p}")

    wanted = None
    if args.profiles:
        wanted = [s.strip() for s in args.profiles.split(",") if s.strip()]

    profiles = gather_profiles_from_files(args.jsonfiles, wanted_profiles=wanted)

    if not profiles:
        raise SystemExit("No profiles found in files or none matched -- aborting.")

    nice_plot(profiles, out_path=str(args.out), curvesmooth_points=args.smooth,
              show_avg=not args.no_avg, flip_axes=args.flip, gridstep=args.gridstep)


if __name__ == "__main__":
    main()

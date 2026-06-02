# -*- coding: utf-8 -*-
"""Generate chart images for the proposal deck."""
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib import font_manager
import os

OUT = os.path.join(os.path.dirname(__file__), "charts")
os.makedirs(OUT, exist_ok=True)

NAVY = "#0B2545"
TEAL = "#13A0A6"
ACCENT = "#F4A300"
GREY = "#8DA9C4"
LIGHT = "#EEF4F7"

plt.rcParams.update({
    "font.family": "DejaVu Sans",
    "axes.edgecolor": "#cccccc",
    "axes.linewidth": 0.8,
    "savefig.dpi": 200,
})

# 1) Year distribution of identified records (n=433)
years = ["2020", "2021", "2022", "2023", "2024", "2025", "2026"]
counts = [1, 31, 38, 34, 62, 159, 108]
fig, ax = plt.subplots(figsize=(7.2, 3.6))
bars = ax.bar(years, counts, color=NAVY)
for b, c in zip(bars, counts):
    ax.text(b.get_x()+b.get_width()/2, c+2, str(c), ha="center", va="bottom",
            fontsize=10, color=NAVY, fontweight="bold")
# highlight SSM era
for i, y in enumerate(years):
    if y in ("2024", "2025", "2026"):
        bars[i].set_color(TEAL)
ax.set_ylabel("Jumlah Artikel", fontsize=10, color=NAVY)
ax.set_title("Distribusi Tahun Publikasi (n = 433 rekaman teridentifikasi)",
             fontsize=12, color=NAVY, fontweight="bold", pad=12)
ax.spines[["top", "right"]].set_visible(False)
ax.set_ylim(0, 180)
ax.tick_params(colors=NAVY)
fig.text(0.66, 0.78, "Ledakan literatur\nSSM/Mamba (2024–2026)",
         fontsize=9, color=TEAL, fontweight="bold")
fig.tight_layout()
fig.savefig(os.path.join(OUT, "years.png"), transparent=True)
plt.close(fig)

# 2) Kappa progression (calibration iterations + final bulk)
iters = ["It.1", "It.2", "It.3", "It.4", "It.5", "It.6\n(PASS)", "Batch\nMassal"]
kappa = [0.00, 0.175, 0.140, 0.00, 0.331, 0.704, 0.710]
fig, ax = plt.subplots(figsize=(7.2, 3.6))
ax.axhspan(0.61, 0.80, color=TEAL, alpha=0.12)
ax.axhline(0.61, color=TEAL, ls="--", lw=1.2)
ax.text(0.05, 0.63, "Ambang 'Substantial' (κ ≥ 0.61)", color=TEAL, fontsize=9, fontweight="bold")
ax.plot(iters, kappa, "-o", color=NAVY, lw=2.4, markersize=8,
        markerfacecolor=ACCENT, markeredgecolor=NAVY)
for x, k in zip(range(len(iters)), kappa):
    ax.text(x, k+0.03, f"{k:.2f}", ha="center", fontsize=9, color=NAVY, fontweight="bold")
ax.set_ylabel("Cohen's Kappa (κ)", fontsize=10, color=NAVY)
ax.set_title("Reliabilitas Antar-Reviewer: Konvergensi Kalibrasi → Substantial",
             fontsize=12, color=NAVY, fontweight="bold", pad=12)
ax.set_ylim(-0.05, 0.9)
ax.spines[["top", "right"]].set_visible(False)
ax.tick_params(colors=NAVY)
fig.tight_layout()
fig.savefig(os.path.join(OUT, "kappa.png"), transparent=True)
plt.close(fig)

# 3) Exclusion reasons donut
labels = ["I-NOMATCH\n(bukan SSM/Mamba)", "P-NOMATCH\n(populasi)", "OTHER",
          "DATE-NOMATCH", "S-NOMATCH"]
sizes = [91, 54, 9, 4, 3]
colors = [NAVY, TEAL, ACCENT, GREY, "#C44536"]
fig, ax = plt.subplots(figsize=(5.4, 4.2))
wedges, _ = ax.pie(sizes, colors=colors, startangle=90,
                   wedgeprops=dict(width=0.42, edgecolor="white", linewidth=2))
ax.legend(wedges, [f"{l}  —  {s}" for l, s in zip(labels, sizes)],
          loc="center", fontsize=8.5, frameon=False, bbox_to_anchor=(0.5, -0.08), ncol=1)
ax.text(0, 0.1, "161", ha="center", fontsize=26, color=NAVY, fontweight="bold")
ax.text(0, -0.18, "dieksklusi", ha="center", fontsize=10, color=GREY)
ax.set_title("Alasan Eksklusi (Modul 5)", fontsize=12, color=NAVY, fontweight="bold")
fig.tight_layout()
fig.savefig(os.path.join(OUT, "exclusion.png"), transparent=True)
plt.close(fig)

print("charts written to", OUT)

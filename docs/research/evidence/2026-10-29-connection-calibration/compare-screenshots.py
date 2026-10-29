"""Compare original same-ICC CDP screenshots; no color-space conversion."""
from pathlib import Path
from PIL import Image
import json
import math

root = Path(__file__).resolve().parent
reference, local, background = [Image.open(root / name) for name in (
    'isolated-flow-07.png', 'local-isolated-screenshot.png', 'isolated-background.png')]
assert reference.size == local.size == background.size
assert reference.info['icc_profile'] == local.info['icc_profile'] == background.info['icc_profile']
differences = []
for a, b, bg in zip(reference.convert('RGB').getdata(), local.convert('RGB').getdata(), background.convert('RGB').getdata()):
    if sum(abs(x-y) for x, y in zip(a, bg)) > 5:
        differences.extend(abs(x-y) for x, y in zip(a, b))
result = {
    'pipeline': 'CDP Page.captureScreenshot for both source and local; original identical ICC; raw encoded RGB comparison',
    'phaseMs': 1745.75,
    'maskPixels': len(differences) // 3,
    'meanAbsoluteRgbError': sum(differences) / len(differences),
    'rmsRgbError': math.sqrt(sum(x*x for x in differences) / len(differences)),
    'maxChannelError': max(differences),
    'exactChannelFraction': differences.count(0) / len(differences),
}
(root / 'same-screenshot-comparison.json').write_text(json.dumps(result, indent=2) + '\n')
print(json.dumps(result, indent=2))

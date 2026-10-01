"""Offline VDA Small execution only; Go owns inputs, processes and publication.

Whole-clip percentile/gamma processing is adapted from BeefTV 1ae25027
tools/depth-capture/depth_capture/pipeline.py (MIT). The imported VDA source and
weights remain fixed Apache-2.0 artifacts; no code or model is downloaded here.
"""
import argparse
import contextlib
import importlib.metadata
import json
import platform
import subprocess
import sys


def emit(phase):
    print(json.dumps({"phase": phase}), flush=True)


def fail(code):
    print(json.dumps({"failure_code": code}), flush=True)
    return 1


def relative_preview(depths, np):
    # Percentiles remain whole-clip facts, as in the fixed source. Pointwise
    # float64 conversion is per frame, avoiding several full-clip float64 copies.
    low, high = np.percentile(depths, [2.0, 98.0])
    if not np.isfinite(low) or not np.isfinite(high) or high <= low:
        return np.zeros(depths.shape, dtype=np.uint8)
    preview = np.empty(depths.shape, dtype=np.uint8)
    for index, frame in enumerate(depths):
        values = np.clip((frame - low) / (high - low), 0.0, 1.0)
        preview[index] = np.rint(np.power(values, 1.25) * 255.0).astype(np.uint8)
    return preview


def run(args):
    required = {"torch": "2.14.0", "torchvision": "0.29.0", "numpy": "2.4.6",
                "opencv-python": "5.0.0.93", "easydict": "1.13", "einops": "0.8.2",
                "matplotlib": "3.11.2", "imageio": "2.37.4", "pillow": "12.3.0",
                "tqdm": "4.70.1"}
    if sys.version_info[:2] != (3, 11):
        return fail("depth_runtime_unavailable")
    versions = {name: importlib.metadata.version(name) for name in required}
    if versions != required:
        return fail("depth_runtime_unavailable")
    import torch
    import numpy as np
    import cv2

    torch.set_num_threads(4)
    if args.device == "mps":
        if not torch.backends.mps.is_available():
            return fail("depth_device_unavailable")
        torch.mps.set_per_process_memory_fraction(0.4)
    elif args.device != "cpu":
        return fail("depth_device_unavailable")
    sys.path.insert(0, args.source)
    # The fixed source prints optional xFormers notices during import. Keep
    # those bounded diagnostics on stderr, apart from the typed event stream.
    with contextlib.redirect_stdout(sys.stderr):
        from video_depth_anything.video_depth import VideoDepthAnything

    emit("loading")
    model = VideoDepthAnything(encoder="vits", features=64, out_channels=[48, 96, 192, 384])
    model.load_state_dict(torch.load(args.model, map_location="cpu", weights_only=True), strict=True)
    model = model.to(args.device).eval()
    capture = cv2.VideoCapture(args.input)
    frames = []
    fps = float(capture.get(cv2.CAP_PROP_FPS))
    try:
        while True:
            ok, frame = capture.read()
            if not ok:
                break
            if len(frames) >= 453 or frame.shape[0] > 960 or frame.shape[1] > 960:
                return fail("depth_budget_exceeded")
            frames.append(cv2.cvtColor(frame, cv2.COLOR_BGR2RGB))
    finally:
        capture.release()
    if not frames or not np.isfinite(fps) or not 0 < fps <= 30.001:
        return fail("depth_input_invalid")
    expected_shape = (len(frames), frames[0].shape[0], frames[0].shape[1])
    inputs = np.stack(frames)
    del frames
    emit("inferring")
    with torch.inference_mode():
        depths, output_fps = model.infer_video_depth(inputs, fps, input_size=280,
                                                   device=args.device, fp32=True)
    del inputs
    if args.device == "mps":
        torch.mps.synchronize()
    if depths.shape != expected_shape or not np.isfinite(depths).all():
        return fail("depth_output_invalid")
    depth_min, depth_max = float(np.min(depths)), float(np.max(depths))
    preview = relative_preview(depths, np)
    del depths
    height, width = preview.shape[1:]
    command = [args.ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
               "-f", "rawvideo", "-pixel_format", "gray", "-video_size", f"{width}x{height}",
               "-framerate", f"{output_fps:.9f}", "-i", "-", "-vf", "scale=1920:1080:flags=bicubic",
               "-an", "-c:v", "libx264", "-crf", "18", "-pix_fmt", "yuv420p",
               "-movflags", "+faststart", "-fs", "536870912", args.output]
    process = subprocess.Popen(command, stdin=subprocess.PIPE)
    try:
        # Emit only after the child exists, so cancellation covers the real tree.
        emit("encoding")
        for frame in preview:
            process.stdin.write(frame.tobytes(order="C"))
        process.stdin.close()
        if process.wait() != 0:
            return fail("depth_output_invalid")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
    print(json.dumps({"complete": True, "device": args.device,
                      "runtime_version": f"python={platform.python_version()};torch={torch.__version__}",
                      "frame_count": int(expected_shape[0]), "fps": output_fps,
                      "depth_min": depth_min, "depth_max": depth_max}), flush=True)
    return 0


def main():
    parser = argparse.ArgumentParser()
    for name in ("input", "output", "source", "model", "device", "ffmpeg"):
        parser.add_argument("--" + name, required=True)
    try:
        return run(parser.parse_args())
    except MemoryError:
        return fail("depth_budget_exceeded")
    except Exception as error:
        # Native messages can expose local files. Go receives only stable codes.
        if "out of memory" in str(error).lower():
            return fail("depth_budget_exceeded")
        if isinstance(error, importlib.metadata.PackageNotFoundError):
            return fail("depth_runtime_unavailable")
        return fail("depth_inference_failed")


if __name__ == "__main__":
    sys.exit(main())

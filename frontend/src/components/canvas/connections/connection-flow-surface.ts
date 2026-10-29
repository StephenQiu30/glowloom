import {
  connectionFlowTexture,
  CONNECTION_FLOW_PERIOD_MS,
} from "./connection-flow-model";

const vertexShader = `#version 300 es
precision highp float;
layout(location=0) in vec2 startPoint;
layout(location=1) in vec2 endPoint;
layout(location=2) in float startDistance;
layout(location=3) in float endDistance;
layout(location=4) in float curveLength;
layout(location=5) in float along;
layout(location=6) in float side;
uniform vec2 viewportSize;
uniform vec2 pan;
uniform float zoom;
out vec2 localPosition;
out vec2 distances;
out float totalLength;
out float segmentLength;
void main() {
  vec2 start = startPoint * zoom + pan;
  vec2 end = endPoint * zoom + pan;
  vec2 delta = end - start;
  float extent = max(length(delta), .0001);
  vec2 tangent = delta / extent;
  vec2 normal = vec2(-tangent.y, tangent.x);
  vec2 local = vec2(mix(-7.0, extent + 7.0, along), side * 7.0);
  vec2 position = start + tangent * local.x + normal * local.y;
  gl_Position = vec4(position.x / viewportSize.x * 2.0 - 1.0, 1.0 - position.y / viewportSize.y * 2.0, 0.0, 1.0);
  localPosition = local;
  distances = vec2(startDistance, endDistance);
  totalLength = curveLength;
  segmentLength = extent;
}`;

const fragmentShader = `#version 300 es
precision highp float;
uniform sampler2D colorRamp;
uniform float phase;
in vec2 localPosition;
in vec2 distances;
in float totalLength;
in float segmentLength;
out vec4 fragment;
void main() {
  float sweep = clamp(totalLength * .55, 120.0, 312.0);
  float head = phase * (totalLength + sweep + 72.0);
  float tail = head - sweep;
  float start = max(max(0.0, tail), distances.x);
  float end = min(min(totalLength, head), distances.y);
  if (end <= start) discard;
  float worldSpan = max(distances.y - distances.x, .0001);
  vec2 visible = (vec2(start, end) - distances.x) / worldSpan * segmentLength;
  float nearest = clamp(localPosition.x, visible.x, visible.y);
  float radiusDistance = length(localPosition - vec2(nearest, 0.0));
  float position = mix(distances.x, distances.y, clamp(localPosition.x / segmentLength, 0.0, 1.0));
  float q = clamp((position - tail) / sweep, 0.0, 1.0);
  float radius = (1.0 + sin(3.141592653589793 * q)) * .5;
  float aa = max(fwidth(radiusDistance), .001);
  float core = 1.0 - smoothstep(radius - aa, radius + aa, radiusDistance);
  float outside = max(0.0, radiusDistance - radius);
  float halo = .22 * exp(-.5 * pow(outside / 1.5, 2.0)) * (1.0 - core);
  vec4 color = texture(colorRamp, vec2(q, .5));
  float alpha = color.a * core + halo;
  if (alpha <= .001) discard;
  fragment = vec4(color.rgb * color.a * core + vec3(0.0, 142.0/255.0, 229.0/255.0) * halo, alpha);
}`;

export type ConnectionFlowView = {
  width: number;
  height: number;
  dpr: number;
  x: number;
  y: number;
  zoom: number;
};

/** One context per canvas. Premultiplied MAX prevents adjoining capsules adding brightness. */
export function createConnectionFlowSurface(canvas: HTMLCanvasElement) {
  const gl = canvas.getContext("webgl2", {
    alpha: true,
    antialias: true,
    depth: false,
    desynchronized: true,
    powerPreference: "high-performance",
    premultipliedAlpha: true,
    preserveDrawingBuffer: false,
    stencil: false,
  });
  if (!gl) throw new Error("Connection animation WebGL2 unavailable");
  const program = gl.createProgram();
  const buffer = gl.createBuffer();
  const array = gl.createVertexArray();
  const texture = gl.createTexture();
  const shaders: WebGLShader[] = [];
  const dispose = () => {
    for (const shader of shaders) gl.deleteShader(shader);
    gl.deleteProgram(program);
    gl.deleteBuffer(buffer);
    gl.deleteVertexArray(array);
    gl.deleteTexture(texture);
  };
  try {
    if (!program || !buffer || !array || !texture)
      throw new Error("Connection animation allocation failed");
    for (const [type, source] of [
      [gl.VERTEX_SHADER, vertexShader],
      [gl.FRAGMENT_SHADER, fragmentShader],
    ] as const) {
      const shader = gl.createShader(type);
      if (!shader) throw new Error("Connection shader allocation failed");
      shaders.push(shader);
      gl.shaderSource(shader, source);
      gl.compileShader(shader);
      if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS))
        throw new Error(
          gl.getShaderInfoLog(shader) ?? "Connection shader compile failed",
        );
      gl.attachShader(program, shader);
    }
    gl.linkProgram(program);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS))
      throw new Error(
        gl.getProgramInfoLog(program) ?? "Connection shader link failed",
      );
    gl.useProgram(program);
    gl.bindVertexArray(array);
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
    let offset = 0;
    [2, 2, 1, 1, 1, 1, 1].forEach((size, index) => {
      gl.enableVertexAttribArray(index);
      gl.vertexAttribPointer(index, size, gl.FLOAT, false, 36, offset * 4);
      offset += size;
    });
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, texture);
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.texImage2D(
      gl.TEXTURE_2D,
      0,
      gl.RGBA8,
      256,
      1,
      0,
      gl.RGBA,
      gl.UNSIGNED_BYTE,
      connectionFlowTexture(),
    );
    const uniforms = {
      size: gl.getUniformLocation(program, "viewportSize"),
      pan: gl.getUniformLocation(program, "pan"),
      zoom: gl.getUniformLocation(program, "zoom"),
      phase: gl.getUniformLocation(program, "phase"),
      ramp: gl.getUniformLocation(program, "colorRamp"),
    };
    gl.uniform1i(uniforms.ramp, 0);
    gl.clearColor(0, 0, 0, 0);
    gl.enable(gl.BLEND);
    gl.blendEquation(gl.MAX);
    gl.blendFunc(gl.ONE, gl.ONE);
    let count = 0;
    return {
      dispose,
      clear: () => gl.clear(gl.COLOR_BUFFER_BIT),
      upload(vertices: Float32Array) {
        gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
        gl.bufferData(gl.ARRAY_BUFFER, vertices, gl.DYNAMIC_DRAW);
        count = vertices.length / 9;
      },
      render(view: ConnectionFlowView, timestamp: number) {
        const width = Math.max(1, Math.round(view.width * view.dpr)),
          height = Math.max(1, Math.round(view.height * view.dpr));
        if (canvas.width !== width) canvas.width = width;
        if (canvas.height !== height) canvas.height = height;
        gl.viewport(0, 0, width, height);
        gl.clear(gl.COLOR_BUFFER_BIT);
        if (!count) return;
        gl.useProgram(program);
        gl.bindVertexArray(array);
        gl.activeTexture(gl.TEXTURE0);
        gl.bindTexture(gl.TEXTURE_2D, texture);
        gl.uniform2f(uniforms.size, view.width, view.height);
        gl.uniform2f(uniforms.pan, view.x, view.y);
        gl.uniform1f(uniforms.zoom, view.zoom);
        gl.uniform1f(
          uniforms.phase,
          (timestamp % CONNECTION_FLOW_PERIOD_MS) / CONNECTION_FLOW_PERIOD_MS,
        );
        gl.drawArrays(gl.TRIANGLES, 0, count);
      },
    };
  } catch (error) {
    dispose();
    throw error;
  }
}

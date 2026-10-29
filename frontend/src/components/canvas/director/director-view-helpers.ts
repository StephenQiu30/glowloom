import * as THREE from "three";
import type { Director } from "./director-model";
/** Editor-only helpers always live outside renderable scene content. */
export function directorNameLabel(name: string) {
  const canvas = document.createElement("canvas");
  canvas.width = 512;
  canvas.height = 64;
  const context = canvas.getContext("2d");
  if (!context) return new THREE.Group();
  context.font = "28px sans-serif";
  context.textAlign = "center";
  context.fillStyle = "#ffffff";
  context.fillText(name, 256, 42, 500);
  const material = new THREE.SpriteMaterial({
    map: new THREE.CanvasTexture(canvas),
    depthTest: false,
  });
  const sprite = new THREE.Sprite(material);
  sprite.scale.set(2, 0.25, 1);
  sprite.position.y = 2.3;
  const group = new THREE.Group();
  group.add(sprite);
  return group;
}
export function directorPathHelpers(scene: Director) {
  const group = new THREE.Group();
  for (const entity of [...scene.objects, ...scene.cameras]) {
    const frames = entity.keyframes
      .filter((frame) => frame.position_keyed !== false)
      .sort((a, b) => a.time - b.time);
    if (frames.length < 2) continue;
    const geometry = new THREE.BufferGeometry().setFromPoints(
      frames.map((frame) =>
        new THREE.Vector3().fromArray(frame.transform.position),
      ),
    );
    group.add(
      new THREE.Line(
        geometry,
        new THREE.LineBasicMaterial({ color: 0x9ca3af }),
      ),
    );
  }
  return group;
}
export function directorHeightReference() {
  const group = new THREE.Group();
  const points = [
    [0, 0, 0],
    [0, 2, 0],
    [-0.1, 2, 0],
    [0.1, 2, 0],
    [0, 2, 0],
    [0, 1, 0],
    [-0.1, 1, 0],
    [0.1, 1, 0],
  ];
  group.add(
    new THREE.Line(
      new THREE.BufferGeometry().setFromPoints(
        points.map((p) => new THREE.Vector3().fromArray(p)),
      ),
      new THREE.LineBasicMaterial({ color: 0x9ca3af }),
    ),
  );
  group.add(directorNameLabel("2 米"));
  return group;
}

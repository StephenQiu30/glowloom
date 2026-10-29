import * as THREE from "three";
import { clone } from "three/addons/utils/SkeletonUtils.js";
import { disposeDirectorObject } from "./director-scene";
/** A viewport-owned cache. Every display owns its geometry, material, texture and skeleton. */
export class DirectorResources {
  private controller = new AbortController();
  private entries = new Map<string, Promise<THREE.Object3D>>();
  async load(
    key: string,
    loader: (signal: AbortSignal) => Promise<THREE.Object3D>,
    signal: AbortSignal,
  ) {
    signal.throwIfAborted();
    this.controller.signal.throwIfAborted();
    let pending = this.entries.get(key);
    if (!pending) {
      pending = loader(this.controller.signal).then((value) => {
        if (this.controller.signal.aborted) {
          disposeDirectorObject(value);
          this.controller.signal.throwIfAborted();
        }
        return value;
      });
      this.entries.set(key, pending);
      const current = pending;
      void pending.catch(() => {
        if (this.entries.get(key) === current) this.entries.delete(key);
      });
    }
    const template = await pending;
    signal.throwIfAborted();
    this.controller.signal.throwIfAborted();
    const model = clone(template);
    const geometries = new Map<THREE.BufferGeometry, THREE.BufferGeometry>(),
      materials = new Map<THREE.Material, THREE.Material>(),
      textures = new Map<THREE.Texture, THREE.Texture>();
    const material = (source: THREE.Material) => {
      let value = materials.get(source);
      if (!value) {
        value = source.clone();
        materials.set(source, value);
        const fields = value as unknown as Record<string, unknown>;
        for (const [name, property] of Object.entries(fields))
          if (property instanceof THREE.Texture) {
            if (!textures.has(property))
              textures.set(property, property.clone());
            fields[name] = textures.get(property);
          }
      }
      return value;
    };
    model.traverse((object) => {
      if (!(object instanceof THREE.Mesh)) return;
      if (!geometries.has(object.geometry))
        geometries.set(object.geometry, object.geometry.clone());
      object.geometry = geometries.get(object.geometry)!;
      object.material = Array.isArray(object.material)
        ? object.material.map(material)
        : material(object.material);
    });
    model.animations = template.animations;
    // Bound abandoned assets without sharing GPU ownership with live clones.
    while (this.entries.size > 32) {
      const oldest = this.entries.entries().next().value!;
      this.entries.delete(oldest[0]);
      void oldest[1].then(disposeDirectorObject, () => {});
    }
    return model;
  }
  dispose() {
    this.controller.abort();
    for (const pending of this.entries.values())
      void pending.then(disposeDirectorObject, () => {});
    this.entries.clear();
  }
}

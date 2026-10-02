// Go gif.EncodeAll fixture: 64×64, red/green/blue, 500ms each, infinite loop.
const encoded =
  "R0lGODlhQABAAAAAACH/C05FVFNDQVBFMi4wAwEAAAAh+QQAMgAAACwAAAAAQABAAIEAAAD/AAAA/wAAAP8CRYyPqcvtD6OctNqLs968+w+G4kiW5omm6sq27gvH8kzX9o3n+s73/g8MCofEovGITCqXzKbzCY1Kp9Sq9YrNarfcrjdQAAAh+QQAMgAAACwAAAAAQABAAIEAAAD/AAAA/wAAAP8CRZSPqcvtD6OctNqLs968+w+G4kiW5omm6sq27gvH8kzX9o3n+s73/g8MCofEovGITCqXzKbzCY1Kp9Sq9YrNarfcrldQAAAh+QQAMgAAACwAAAAAQABAAIEAAAD/AAAA/wAAAP8CRZyPqcvtD6OctNqLs968+w+G4kiW5omm6sq27gvH8kzX9o3n+s73/g8MCofEovGITCqXzKbzCY1Kp9Sq9YrNarfcrndQAAA7";

export function animatedGIFBytes() {
  return Uint8Array.from(atob(encoded), (character) => character.charCodeAt(0));
}

export function animatedGIFFile(name = "动画.gif") {
  return new File([animatedGIFBytes()], name, { type: "image/gif" });
}

import Image from "next/image";
import {
  Aperture,
  Camera,
  ChevronLeft,
  ChevronRight,
  Circle,
  Focus,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { cinemaCatalog, type CinemaCamera } from "./cinema-kit-model";
import { CinemaPreview, cinemaPreviewAsset } from "./cinema-preview";

const cameraGroups = [
  { key: "body", label: "相机", choices: cinemaCatalog.camera, Icon: Camera },
  { key: "lens", label: "镜头", choices: cinemaCatalog.lens, Icon: Focus },
  {
    key: "focal_length",
    label: "焦距",
    choices: cinemaCatalog.focalLength,
    Icon: Circle,
  },
  {
    key: "aperture",
    label: "光圈",
    choices: cinemaCatalog.aperture,
    Icon: Aperture,
  },
] as const;

export function CinemaCameraPicker({
  value,
  onChange,
}: {
  value: CinemaCamera;
  onChange: (value: CinemaCamera) => void;
}) {
  return (
    <div className="cinema-camera-content">
      <div className="cinema-camera-preview">
        <CinemaPreview kind="camera" name={value.body} />
      </div>
      <div className="cinema-camera-columns">
        {cameraGroups.map(({ key, label, choices, Icon }) => {
          const index = choices.indexOf(value[key]);
          const step = (offset: number) =>
            choices[(index + offset + choices.length) % choices.length];
          return (
            <div className="cinema-camera-column" key={key}>
              <h3>{label}</h3>
              <span className="cinema-camera-neighbor" aria-hidden="true">
                {step(-1)}
              </span>
              <ToggleGroup
                type="single"
                value={value[key]}
                onValueChange={(choice) => {
                  if (choice) onChange({ ...value, [key]: choice });
                }}
                aria-label={label}
                className="cinema-camera-options"
                onKeyDown={(event) => {
                  const offset = {
                    ArrowLeft: -1,
                    ArrowUp: -1,
                    ArrowRight: 1,
                    ArrowDown: 1,
                  }[event.key];
                  if (offset) {
                    event.preventDefault();
                    onChange({ ...value, [key]: step(offset) });
                  }
                }}
              >
                {choices.map((choice) => {
                  const asset = cinemaPreviewAsset(key, choice);
                  return (
                    <ToggleGroupItem
                      value={choice}
                      key={choice}
                      aria-label={choice}
                      hidden={choice !== value[key]}
                      className="cinema-camera-choice"
                    >
                      {asset ? (
                        <Image
                          unoptimized
                          className="cinema-camera-icon"
                          src={asset.image}
                          alt=""
                          width={48}
                          height={40}
                          sizes="48px"
                        />
                      ) : (
                        <Icon className="size-10" />
                      )}
                      <span>{choice}</span>
                    </ToggleGroupItem>
                  );
                })}
              </ToggleGroup>
              <span className="cinema-camera-neighbor" aria-hidden="true">
                {step(1)}
              </span>
              <div className="cinema-camera-arrows">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`上一${label}`}
                  onClick={() => onChange({ ...value, [key]: step(-1) })}
                >
                  <ChevronLeft />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`下一${label}`}
                  onClick={() => onChange({ ...value, [key]: step(1) })}
                >
                  <ChevronRight />
                </Button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

import { LayoutGrid, List, Search, X } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { CanvasToolbarButton } from "./canvas-toolbar-primitives.tsx";

export type CanvasAssetLayout = "grid" | "list";

export function CanvasAssetLayoutButton({
  layout,
  onChange,
}: {
  layout: CanvasAssetLayout;
  onChange: (layout: CanvasAssetLayout) => void;
}) {
  const Icon = layout === "grid" ? LayoutGrid : List;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <CanvasToolbarButton aria-label="资产显示方式" title="资产显示方式">
          <Icon />
        </CanvasToolbarButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" data-canvas-no-zoom>
        <DropdownMenuRadioGroup
          value={layout}
          onValueChange={(value) => {
            if (value === "grid" || value === "list") onChange(value);
          }}
        >
          <DropdownMenuRadioItem value="grid">
            <LayoutGrid />
            网格
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="list">
            <List />
            列表
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function CanvasAssetSearch({
  label,
  value,
  onChange,
  onSubmit,
  onClear,
  disabled,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  onClear: () => void;
  disabled?: boolean;
}) {
  return (
    <form
      className="min-w-0 flex-1"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit?.();
      }}
    >
      <InputGroup className="canvas-asset-search">
        <InputGroupAddon>
          {onSubmit ? (
            <CanvasToolbarButton
              type="submit"
              aria-label="搜索"
              title={label}
              disabled={disabled}
            >
              <Search />
            </CanvasToolbarButton>
          ) : (
            <Search aria-hidden="true" />
          )}
        </InputGroupAddon>
        <InputGroupInput
          type="search"
          aria-label={label}
          placeholder={label}
          value={value}
          disabled={disabled}
          onChange={(event) => onChange(event.target.value)}
        />
        {value && (
          <InputGroupAddon align="inline-end">
            <CanvasToolbarButton
              aria-label="清空搜索"
              title="清空搜索"
              disabled={disabled}
              onClick={onClear}
            >
              <X />
            </CanvasToolbarButton>
          </InputGroupAddon>
        )}
      </InputGroup>
    </form>
  );
}

/** UI-only fixture: deterministic capabilities, no account, save, upload or generation requests. */
import { TooltipProvider } from "../../src/components/ui/tooltip";
import { Provider } from "react-redux";
import { makeAppStore } from "../../src/components/canvas/runtime/store/app-store";
import { AppFeedbackProvider } from "../../src/components/canvas/runtime/components/feedback/app-feedback-provider";
import { useState } from "react";
import { createRoot } from "react-dom/client";
import { SWRConfig, unstable_serialize } from "swr";
import { ThemeProvider } from "next-themes";
import { CanvasDocumentNodePanel } from "../../src/components/canvas/nodes/canvas-document-node-panel";
import { CanvasAccountContext } from "../../src/components/canvas/runtime/components/account/account-provider";
import {
  CanvasNodeType,
  type CanvasNodeData,
} from "../../src/components/canvas/types/canvas";
import { toCanvasGraph } from "../../src/components/canvas/lib/canvas-document-adapter";
import { RichTextEditor } from "../../src/components/canvas/runtime/components/editor/rich-text/rich-text-editor";
import type { RichTextDocument } from "../../src/components/canvas/runtime/components/editor/types/rich-text";
import "../../src/app/globals.css";
import "../../src/components/canvas/canvas.css";

const store = makeAppStore("dark");
const image: CanvasNodeData = {
  id: "image-fixture",
  type: CanvasNodeType.Image,
  title: "图片节点交互验证",
  position: { x: 0, y: 0 },
  width: 320,
  height: 320,
  metadata: { prompt: "描述画面，保留引用与参数。" },
};
image.documentData = {
  ...toCanvasGraph({
    nodes: [image],
    connections: [],
    backgroundMode: "dots",
    showImageInfo: false,
    viewport: { x: 0, y: 0, k: 1 },
  }).nodes[0],
  parameters: {
    modelSelection: { connectionId: "fixture", modelId: "fixture" },
    modelParameters: { size: "2K", count: 1 },
  },
};
const reference: CanvasNodeData = {
  id: "text-reference-fixture",
  type: CanvasNodeType.Text,
  title: "参考文本",
  position: { x: 0, y: 0 },
  width: 320,
  height: 320,
  metadata: { content: "用于验证的文本参考" },
};
const fallback = {
  [unstable_serialize(["canvas-node-models", "fixture:", "image"])]: [
    {
      connectionId: "fixture",
      modelId: "fixture",
      modelName: "UI 验证模型",
      imageSizes: ["1K", "2K"],
      nativeCapabilities: { protocols: ["ark_image"] },
    },
  ],
  [unstable_serialize([
    "canvas-generation-options",
    "fixture:",
    "image",
    "fixture",
    "fixture",
  ])]: {
    parameters: [
      {
        key: "size",
        label: "清晰度",
        values: ["1K", "2K"],
        defaultValue: "2K",
      },
      {
        key: "count",
        label: "生成数量",
        values: ["1", "2", "3", "4"],
        defaultValue: "1",
        numeric: true,
      },
    ],
  },
};
function Fixture() {
  const [node, setNode] = useState(image);
  const [text, setText] = useState("全屏文档与格式菜单验证");
  const [document, setDocument] = useState<RichTextDocument>();
  const [connected, setConnected] = useState<CanvasNodeData[]>([]);
  return (
    <ThemeProvider attribute="class" forcedTheme="dark">
      <Provider store={store}>
        <AppFeedbackProvider>
          <TooltipProvider>
            <SWRConfig
              value={{
                fallback,
                isPaused: () => true,
                revalidateOnMount: false,
              }}
            >
              <CanvasAccountContext
                value={{
                  account: { id: "fixture" },
                  ready: true,
                  signingOut: false,
                  refresh: async () => undefined,
                  adopt: () => {},
                  leave: async () => {},
                  signOut: async () => {},
                }}
              >
                <main className="mx-auto grid max-w-3xl gap-6 p-6">
                  <h1>画布节点编辑隔离验证</h1>
                  <p>
                    仅验证正式组件的展示与交互。模型能力为测试数据，没有保存、上传或生成请求。
                  </p>
                  <section className="min-w-0">
                    <CanvasDocumentNodePanel
                      node={node}
                      onChange={setNode}
                      onClose={() => {}}
                      nodes={[node, reference]}
                      connectedNodes={connected}
                      onStartReferenceSelection={() =>
                        setConnected([reference])
                      }
                      onDisconnectReference={() => setConnected([])}
                    />
                  </section>
                  <section className="h-80 overflow-hidden rounded-xl border">
                    <RichTextEditor
                      documentTitle="测试文档"
                      value={text}
                      document={document}
                      documentMode
                      onChange={(value, doc) => {
                        setText(value);
                        setDocument(doc);
                      }}
                    />
                  </section>
                  <details>
                    <summary>本页内存参数（非保存回执）</summary>
                    <pre className="overflow-auto" aria-label="本页节点数据">
                      {JSON.stringify(node, null, 2)}
                    </pre>
                  </details>
                </main>
              </CanvasAccountContext>
            </SWRConfig>
          </TooltipProvider>
        </AppFeedbackProvider>
      </Provider>
    </ThemeProvider>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);

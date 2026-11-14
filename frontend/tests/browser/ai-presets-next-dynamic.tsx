/** Vite fixture adapter for Next's client-only lazy component boundary. */
import { lazy, Suspense, type ComponentType } from "react";
export default function dynamic<P extends object>(
  loader: () => Promise<ComponentType<P>>,
  options?: { loading?: ComponentType },
) {
  const Component = lazy(() =>
    loader().then((defaultExport) => ({ default: defaultExport })),
  );
  const Loading = options?.loading;
  return function FixtureDynamic(props: P) {
    return (
      <Suspense fallback={Loading ? <Loading /> : null}>
        <Component {...props} />
      </Suspense>
    );
  };
}

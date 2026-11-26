import { clsx } from "clsx";
import { X } from "lucide-react";
import { createContext, type ReactNode, useCallback, useContext, useState } from "react";

type Kind = "success" | "error";
type Toast = { id: number; message: string; kind: Kind };
type Notify = (message: string, kind?: Kind) => void;

const ToastContext = createContext<Notify>(() => {});

export const useNotify = () => useContext(ToastContext);

let nextId = 1;

/**
 * Short notifications ("保存しました"). Messages are announced through live regions
 * and stay until closed or replaced, so nobody has to read them against a timer.
 */
export function Toaster({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const notify = useCallback<Notify>((message, kind = "success") => {
    setToasts((ts) => [...ts.slice(-2), { id: nextId++, message, kind }]);
  }, []);
  const close = (id: number) => setToasts((ts) => ts.filter((t) => t.id !== id));

  return (
    <ToastContext.Provider value={notify}>
      {children}
      <div className="pointer-events-none fixed right-4 bottom-4 z-50 flex w-[min(92vw,24rem)] flex-col gap-2">
        {(["success", "error"] as const).map((kind) => (
          <div
            key={kind}
            role={kind === "error" ? "alert" : "status"}
            className="flex flex-col gap-2"
          >
            {toasts
              .filter((t) => t.kind === kind)
              .map((t) => (
                <div
                  key={t.id}
                  className={clsx(
                    "pointer-events-auto flex items-start justify-between gap-3 rounded-md border p-3 shadow-md",
                    kind === "error"
                      ? "border-red-700 bg-red-50 text-red-900"
                      : "border-green-700 bg-green-50 text-green-900",
                  )}
                >
                  <span>{t.message}</span>
                  <button
                    type="button"
                    onClick={() => close(t.id)}
                    aria-label="通知を閉じる"
                    className="rounded p-0.5 hover:bg-black/10"
                  >
                    <X aria-hidden="true" size={18} />
                  </button>
                </div>
              ))}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

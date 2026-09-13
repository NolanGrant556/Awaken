import { useEffect, useRef, useState, type FormEvent } from "react";
import { AwakenIcon } from "@awaken/design-system";
import {
  EMPTY_EMBEDDED_BROWSER_STATE,
  normalizeBrowserAddressInput,
  type EmbeddedBrowserBridge,
  type EmbeddedBrowserState,
  type EmbeddedBrowserTabRef,
} from "../../shared/embedded-browser";

export type EmbeddedBrowserClient = Pick<
  EmbeddedBrowserBridge,
  | "navigate"
  | "back"
  | "forward"
  | "reload"
  | "stop"
  | "close"
  | "getState"
  | "setViewport"
  | "setActive"
  | "onStateChanged"
>;

export type EmbeddedHtmlNavigation = {
  canGoBack: boolean;
  canGoForward: boolean;
  address: string;
  loading: boolean;
  goBack: () => void;
  goForward: () => void;
  reload: () => void;
};

type Props = {
  client: EmbeddedBrowserClient;
  viewRef: EmbeddedBrowserTabRef;
  active: boolean;
  htmlOccupied?: boolean;
  onLeaveHtmlPreview?: () => void;
  htmlNavigation?: EmbeddedHtmlNavigation;
  children?: React.ReactNode;
};

function chromeCoveredByModal(): boolean {
  return Boolean(document.querySelector('[aria-modal="true"]'));
}

export default function EmbeddedBrowserChrome({
  client,
  viewRef,
  active,
  htmlOccupied = false,
  onLeaveHtmlPreview,
  htmlNavigation,
  children,
}: Props) {
  const stripRef = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<EmbeddedBrowserState>(EMPTY_EMBEDDED_BROWSER_STATE);
  const [address, setAddress] = useState("");
  const editingRef = useRef(false);
  const [addressError, setAddressError] = useState<string | undefined>();

  useEffect(() => {
    editingRef.current = false;
    setAddressError(undefined);
    void client.getState(viewRef).then((next) => {
      setState(next);
      if (!editingRef.current) setAddress(next.url);
    });
    return client.onStateChanged((payload) => {
      if (payload.viewId !== viewRef.viewId || payload.threadId !== viewRef.threadId) return;
      setState(payload.state);
      if (!editingRef.current) setAddress(payload.state.url);
    });
  }, [client, viewRef.threadId, viewRef.viewId]);

  useEffect(() => {
    if (editingRef.current || state.hasPage || !htmlNavigation) return;
    setAddress(htmlNavigation.address);
  }, [htmlNavigation, state.hasPage]);

  useEffect(() => {
    if (!active) {
      void client.setActive(null);
      return;
    }
    void client.setActive(viewRef);
    return () => {
      void client.setActive(null);
    };
  }, [active, client, viewRef.threadId, viewRef.viewId]);

  const showPage = active && state.hasPage && !htmlOccupied;
  useEffect(() => {
    if (!showPage) {
      void client.setViewport(viewRef, null);
      return;
    }
    let frame = 0;
    const report = () => {
      if (chromeCoveredByModal()) {
        void client.setViewport(viewRef, null);
      } else {
        const rect = stripRef.current?.getBoundingClientRect();
        void client.setViewport(
          viewRef,
          rect
            ? { x: rect.x, y: rect.y, width: rect.width, height: rect.height }
            : null,
        );
      }
      frame = requestAnimationFrame(report);
    };
    frame = requestAnimationFrame(report);
    return () => {
      cancelAnimationFrame(frame);
      void client.setViewport(viewRef, null);
    };
  }, [client, showPage, viewRef.threadId, viewRef.viewId]);

  const showingHtml = htmlOccupied || (Boolean(htmlNavigation) && !state.hasPage);
  const toolbarCanGoBack = showingHtml
    ? Boolean(htmlNavigation?.canGoBack)
    : state.canGoBack;
  const toolbarCanGoForward = showingHtml
    ? Boolean(htmlNavigation?.canGoForward)
    : state.canGoForward;
  const toolbarLoading = showingHtml
    ? Boolean(htmlNavigation?.loading)
    : state.loading;

  const submitAddress = (event: FormEvent) => {
    event.preventDefault();
    const parsed = normalizeBrowserAddressInput(address);
    if (!parsed.ok) {
      setAddressError(
        parsed.reason === "unsupported_scheme"
          ? "只支持 http 或 https 地址"
          : parsed.reason === "empty"
            ? "请输入网址"
            : "地址无效",
      );
      return;
    }
    setAddressError(undefined);
    editingRef.current = false;
    onLeaveHtmlPreview?.();
    void client.navigate(viewRef, parsed.url);
  };

  return (
    <div className="embedded-browser">
      <form className="embedded-browser__toolbar" onSubmit={submitAddress}>
        <button
          type="button"
          aria-label="后退"
          title="后退"
          disabled={!toolbarCanGoBack}
          onClick={() =>
            showingHtml ? htmlNavigation?.goBack() : void client.back(viewRef)
          }
        >
          <AwakenIcon concept="navigate-back" size={16} />
        </button>
        <button
          type="button"
          aria-label="前进"
          title="前进"
          disabled={!toolbarCanGoForward}
          onClick={() =>
            showingHtml
              ? htmlNavigation?.goForward()
              : void client.forward(viewRef)
          }
        >
          <AwakenIcon concept="forward" size={16} />
        </button>
        <button
          type="button"
          aria-label={toolbarLoading ? "停止加载" : "刷新页面"}
          title={toolbarLoading ? "停止加载" : "刷新页面"}
          onClick={() => {
            if (showingHtml) {
              htmlNavigation?.reload();
              return;
            }
            void (state.loading ? client.stop(viewRef) : client.reload(viewRef));
          }}
        >
          <AwakenIcon concept={toolbarLoading ? "close" : "refresh"} size={16} />
        </button>
        <input
          className="embedded-browser__address"
          aria-label="当前地址"
          placeholder="输入网址，或从文件树打开 HTML"
          value={address}
          onChange={(event) => {
            editingRef.current = true;
            setAddressError(undefined);
            setAddress(event.target.value);
          }}
          onFocus={() => {
            editingRef.current = true;
          }}
          onBlur={() => {
            editingRef.current = false;
            if (!address.trim()) {
              setAddress(showingHtml ? (htmlNavigation?.address ?? "") : state.url);
            }
          }}
        />
      </form>
      {addressError ? (
        <p className="embedded-browser__error" role="alert">
          {addressError}
        </p>
      ) : null}
      {showPage ? (
        <div ref={stripRef} className="embedded-browser__page" />
      ) : (
        <div className="embedded-browser__page">
          {children ?? (
            <div className="project-files-preview-empty is-centered">
              <AwakenIcon concept="browser" size={24} />
              <strong>浏览器</strong>
              <span>输入网址，或从文件树打开 HTML 文件</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

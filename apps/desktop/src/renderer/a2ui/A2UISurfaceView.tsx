import { AwakenButton as Button } from "@awaken/design-system";
import type { ReactNode } from "react";
import { parseA2UISurface, type A2UIComponent } from "./surface";

type A2UISurfaceViewProps = {
  title: string;
  content?: string;
};

type ComponentMap = Map<string, A2UIComponent>;

function badgeClass(tone?: string): string {
  switch (tone) {
    case "safe":
    case "danger":
      return `a2ui-badge a2ui-badge-${tone}`;
    default:
      return "a2ui-badge a2ui-badge-warning";
  }
}

function textClass(variant?: string): string {
  switch (variant) {
    case "title":
      return "a2ui-text a2ui-text-title";
    case "caption":
      return "a2ui-text a2ui-text-caption";
    default:
      return "a2ui-text";
  }
}

function renderChildren(ids: string[] | undefined, components: ComponentMap, seen: Set<string>) {
  if (!ids?.length) {
    return null;
  }
  return ids.map((id) => renderComponent(id, components, seen));
}

function buttonLabel(component: A2UIComponent, components: ComponentMap): string {
  if (component.text) {
    return component.text;
  }
  if (component.child) {
    return components.get(component.child)?.text || component.child;
  }
  return component.action?.event?.name || "Action";
}

function renderComponent(id: string, components: ComponentMap, seen: Set<string>): ReactNode {
  const component = components.get(id);
  if (!component) {
    return (
      <div key={id} className="a2ui-unsupported">
        Missing component: {id}
      </div>
    );
  }
  if (seen.has(id)) {
    return (
      <div key={id} className="a2ui-unsupported">
        Circular reference: {id}
      </div>
    );
  }

  const nextSeen = new Set(seen);
  nextSeen.add(id);

  switch (component.component) {
    case "Card":
      return (
        <div key={id} className="a2ui-card">
          {renderChildren(component.children, components, nextSeen)}
        </div>
      );
    case "Column":
      return (
        <div key={id} className="a2ui-column">
          {renderChildren(component.children, components, nextSeen)}
        </div>
      );
    case "Row":
      return (
        <div key={id} className="a2ui-row">
          {renderChildren(component.children, components, nextSeen)}
        </div>
      );
    case "Text":
      return (
        <p key={id} className={textClass(component.variant)}>
          {component.text}
        </p>
      );
    case "Badge":
      return (
        <span key={id} className={badgeClass(component.tone)}>
          {component.text}
        </span>
      );
    case "KeyValue":
      return (
        <div key={id} className="a2ui-key-value">
          <span>{component.label}</span>
          <strong>{component.value}</strong>
        </div>
      );
    case "Button":
      return (
        <Button
          key={id}
          size="sm"
          variant={component.action?.event?.name === "approval.approve" ? "primary" : "default"}
          disabled
          title={component.action?.event?.name}
        >
          {buttonLabel(component, components)}
        </Button>
      );
    default:
      return (
        <div key={id} className="a2ui-unsupported">
          Unsupported component: {component.component}
        </div>
      );
  }
}

export function A2UISurfaceView({ title, content }: A2UISurfaceViewProps) {
  const result = parseA2UISurface(content);
  if (!result.ok) {
    return (
      <div className="a2ui-shell">
        <div className="a2ui-header">
          <h3>{title}</h3>
          <span>invalid</span>
        </div>
        <p className="a2ui-error">{result.error}</p>
      </div>
    );
  }

  const components = new Map(
    result.surface.updateComponents.components.map((component) => [component.id, component]),
  );
  const rootId = components.has("root")
    ? "root"
    : result.surface.updateComponents.components[0]?.id;

  return (
    <div className="a2ui-shell">
      <div className="a2ui-header">
        <h3>{title}</h3>
        <span>{result.surface.version}</span>
      </div>
      <div className="a2ui-meta">
        <span>{result.surface.updateComponents.surfaceId}</span>
        <span>{result.surface.metadata?.source || "unknown source"}</span>
        <span>{result.surface.metadata?.readOnly ? "read-only" : "interactive"}</span>
      </div>
      {rootId ? renderComponent(rootId, components, new Set()) : null}
    </div>
  );
}

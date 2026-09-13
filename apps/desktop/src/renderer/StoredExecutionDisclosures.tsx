import { AwakenIcon } from "@awaken/design-system";
import { memo, useCallback } from "react";
import {
  AwakenDisclosure as Disclosure,
  AwakenToolTraceGroup as ToolTraceGroup,
  AwakenToolTraceLine as ToolTraceLine,
  AwakenThoughtTrace as ThoughtTrace,
  type AwakenDisclosureProps,
  type ToolTraceAction,
  type AwakenToolTraceGroupProps,
  type AwakenToolTraceLineProps,
} from "@awaken/design-system";
import {
  useWorkbenchDisclosure,
  type WorkbenchDisclosureStore,
} from "./workbenchDisclosureStore";

export function StoredThoughtTrace({
  disclosureKey,
  store,
  ...props
}: React.ComponentProps<typeof ThoughtTrace> & {
  disclosureKey: string;
  store: WorkbenchDisclosureStore;
}) {
  const [open, setOpen] = useWorkbenchDisclosure(store, disclosureKey);
  return (
    <ThoughtTrace {...props} open={open ?? false} onOpenChange={setOpen} />
  );
}

export type StoredToolTraceLineProps = Omit<
  AwakenToolTraceLineProps,
  "open" | "onOpenChange"
> & {
  disclosureKey: string;
  store: WorkbenchDisclosureStore;
};

export const StoredToolTraceLine = memo(function StoredToolTraceLine({
  disclosureKey,
  store,
  ...props
}: StoredToolTraceLineProps) {
  const [open, setOpen] = useWorkbenchDisclosure(store, disclosureKey);
  return <ToolTraceLine {...props} open={open} onOpenChange={setOpen} />;
});

export type StoredToolTraceGroupProps = Omit<
  AwakenToolTraceGroupProps,
  "open" | "onOpenChange" | "renderAction"
> & {
  disclosureKey: string;
  replayed: boolean;
  store: WorkbenchDisclosureStore;
};

export function storedToolTraceGroupPropsEqual(
  previous: StoredToolTraceGroupProps,
  next: StoredToolTraceGroupProps,
): boolean {
  return (
    previous.disclosureKey === next.disclosureKey &&
    previous.replayed === next.replayed &&
    previous.store === next.store &&
    previous.label === next.label &&
    previous.detail === next.detail &&
    previous.defaultOpen === next.defaultOpen &&
    previous.icon === next.icon &&
    previous.animateItems === next.animateItems &&
    previous.children === next.children &&
    previous.entryCount === next.entryCount &&
    previous.currentActivity === next.currentActivity &&
    previous.attention === next.attention &&
    previous.activityStatus === next.activityStatus &&
    previous.onSelectAction === next.onSelectAction &&
    previous.actions.length === next.actions.length &&
    previous.actions.every((action, index) => action === next.actions[index])
  );
}

export const StoredToolTraceGroup = memo(function StoredToolTraceGroup({
  actions,
  disclosureKey,
  replayed,
  store,
  ...props
}: StoredToolTraceGroupProps) {
  const [open, setOpen] = useWorkbenchDisclosure(store, disclosureKey);
  const renderAction = useCallback(
    (action: ToolTraceAction, lineProps: AwakenToolTraceLineProps) => {
      const storedLineProps = { ...lineProps };
      delete storedLineProps.open;
      delete storedLineProps.onOpenChange;
      return (
        <StoredToolTraceLine
          {...storedLineProps}
          disclosureKey={`${disclosureKey}:action:${action.id}`}
          store={store}
        />
      );
    },
    [disclosureKey, store],
  );
  return (
    <ToolTraceGroup
      {...props}
      actions={actions}
      open={open}
      onOpenChange={setOpen}
      renderAction={renderAction}
    />
  );
}, storedToolTraceGroupPropsEqual);

export type StoredExecutionProcessDisclosureProps = Pick<
  AwakenDisclosureProps,
  "title" | "children"
> & {
  disclosureKey: string;
  store: WorkbenchDisclosureStore;
};

export const StoredExecutionProcessDisclosure = memo(
  function StoredExecutionProcessDisclosure({
    children,
    disclosureKey,
    store,
    title,
  }: StoredExecutionProcessDisclosureProps) {
    const [open, setOpen] = useWorkbenchDisclosure(store, disclosureKey);
    return (
      <div
        className={`execution-process-disclosure${
          open ? " execution-process-disclosure-open" : ""
        }`}
      >
        <Disclosure open={open} onOpenChange={setOpen} title={title}>
          {children}
        </Disclosure>
      </div>
    );
  },
);

export const SEARCH_TRACE_GROUP_ICON = (
  <AwakenIcon concept="search" size={14} />
);

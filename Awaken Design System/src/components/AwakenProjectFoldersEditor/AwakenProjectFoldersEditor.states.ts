import type { ComponentProps } from "react";
import type { AwakenProjectFoldersEditor } from "./AwakenProjectFoldersEditor";

export type AwakenProjectFoldersEditorState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenProjectFoldersEditor>>;
};

const noop = () => {};

export const AwakenProjectFoldersEditorStates: AwakenProjectFoldersEditorState[] = [
  {
    id: "empty",
    label: "尚未选择文件夹",
    props: {
      folders: [],
      emptyHint: "可以先不选文件夹。项目会直接创建，之后随时能加。",
      onAddFolder: noop,
    },
  },
  {
    id: "single-primary",
    label: "单个主文件夹",
    props: {
      folders: [
        {
          path: "/Users/awaken/Projects/awaken",
          isPrimary: true,
          onRemove: noop,
        },
      ],
      onAddFolder: noop,
    },
  },
  {
    id: "draft-multi-folder",
    label: "创建项目：切主与移出",
    props: {
      folders: [
        {
          path: "/Users/awaken/Projects/awaken",
          isPrimary: true,
          onRemove: noop,
        },
        {
          path: "/Users/awaken/Projects/awaken-design-system",
          onMakePrimary: noop,
          onRemove: noop,
        },
        {
          path: "/Users/awaken/Projects/awaken-docs",
          onMakePrimary: noop,
          onRemove: noop,
        },
      ],
      onAddFolder: noop,
    },
  },
  {
    id: "edit-with-extra-action",
    label: "编辑项目：额外带废纸篓菜单",
    props: {
      folders: [
        {
          path: "/Users/awaken/Projects/awaken",
          isPrimary: true,
          onRemove: noop,
          actions: [{ id: "trash", label: "移到废纸篓", tone: "danger", onSelect: noop }],
        },
        {
          path: "/Users/awaken/Projects/awaken-design-system",
          onMakePrimary: noop,
          onRemove: noop,
          actions: [{ id: "trash", label: "移到废纸篓", tone: "danger", onSelect: noop }],
        },
      ],
      onAddFolder: noop,
    },
  },
  {
    id: "windows-paths",
    label: "Windows 反斜杠路径",
    props: {
      folders: [
        {
          path: "C:\\Users\\awaken\\Documents\\09_经营管理部",
          isPrimary: true,
          onRemove: noop,
        },
        {
          path: "C:\\Users\\awaken\\Documents\\04_市场部",
          onMakePrimary: noop,
          onRemove: noop,
        },
      ],
      onAddFolder: noop,
    },
  },
  {
    id: "busy-row",
    label: "单行正在提交",
    props: {
      folders: [
        {
          path: "/Users/awaken/Projects/awaken",
          isPrimary: true,
          onRemove: noop,
        },
        {
          path: "/Users/awaken/Projects/awaken-design-system",
          onMakePrimary: noop,
          onRemove: noop,
        },
      ],
      busyPath: "/Users/awaken/Projects/awaken-design-system",
      onAddFolder: noop,
    },
  },
];

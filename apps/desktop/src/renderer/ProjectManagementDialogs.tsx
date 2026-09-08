import {
  AwakenIcon,
  AwakenButton as Button,
  AwakenDialog as Dialog,
  AwakenProjectFoldersEditor as ProjectFoldersEditor,
  AwakenSelectableCard as SelectableCard,
} from "@awaken/design-system";
import { shouldSubmitProjectNameInputKey } from "./projectRunScope";
import { isSameWorkspacePath } from "./projectWorkspace";
import { useProjectOperations } from "./useProjectOperations";

type Props = {
  projectOperations: Pick<
    ReturnType<typeof useProjectOperations>,
    | "createProjectDialogOpen"
    | "setCreateProjectDialogOpen"
    | "pendingProjectMode"
    | "setPendingProjectMode"
    | "pickingWorkspace"
    | "draftProjectFolders"
    | "draftPrimaryFolder"
    | "setDraftPrimaryFolder"
    | "draftProjectName"
    | "setDraftProjectName"
    | "createProjectSubmitting"
    | "editingProjectId"
    | "editingProjectName"
    | "setEditingProjectName"
    | "projectRenameSubmitting"
    | "projectFolderBusyPath"
    | "pendingProjectDelete"
    | "setPendingProjectDelete"
    | "projectDeleteSubmitting"
    | "closeProjectEditor"
    | "handleFinishEditingProject"
    | "handleConfirmProjectDelete"
    | "handleAddDraftProjectFolder"
    | "handleRemoveDraftProjectFolder"
    | "handleCreateProject"
    | "handleAddProjectFolder"
    | "projectFolderRows"
    | "projectDeleteDialogTitle"
    | "projectDeleteDialogBody"
    | "projectDeleteConfirmLabel"
  >;
};
export function ProjectManagementDialogs({ projectOperations }: Props) {
  const {
    createProjectDialogOpen,
    setCreateProjectDialogOpen,
    pendingProjectMode,
    setPendingProjectMode,
    pickingWorkspace,
    draftProjectFolders,
    draftPrimaryFolder,
    setDraftPrimaryFolder,
    draftProjectName,
    setDraftProjectName,
    createProjectSubmitting,
    editingProjectId,
    editingProjectName,
    setEditingProjectName,
    projectRenameSubmitting,
    projectFolderBusyPath,
    pendingProjectDelete,
    setPendingProjectDelete,
    projectDeleteSubmitting,
    closeProjectEditor,
    handleFinishEditingProject,
    handleConfirmProjectDelete,
    handleAddDraftProjectFolder,
    handleRemoveDraftProjectFolder,
    handleCreateProject,
    handleAddProjectFolder,
    projectFolderRows,
    projectDeleteDialogTitle,
    projectDeleteDialogBody,
    projectDeleteConfirmLabel,
  } = projectOperations;
  return (
    <>
      <Dialog
        open={createProjectDialogOpen}
        title="创建项目"
        onOpenChange={(open) => {
          if (!open && createProjectSubmitting) {
            return;
          }
          setCreateProjectDialogOpen(open);
        }}
        footer={
          <>
            <div className="create-project-dialog-source">
              <SelectableCard
                compact
                selected={pendingProjectMode === "local"}
                icon={<AwakenIcon concept="theme-system" size={16} />}
                title="Local"
                onSelect={() => setPendingProjectMode("local")}
              />
            </div>
            <Button
              variant="default"
              size="md"
              disabled={createProjectSubmitting}
              onClick={() => setCreateProjectDialogOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={createProjectSubmitting}
              onClick={() => {
                void handleCreateProject();
              }}
            >
              创建项目
            </Button>
          </>
        }
      >
        <div className="create-project-dialog-body">
          <label className="create-project-dialog-field">
            <span className="create-project-dialog-label">项目名称</span>
            <input
              type="text"
              className="create-project-dialog-input"
              value={draftProjectName}
              onChange={(event) => setDraftProjectName(event.target.value)}
              onKeyDown={(event) => {
                if (
                  shouldSubmitProjectNameInputKey({
                    key: event.key,
                    isComposing: event.nativeEvent.isComposing,
                  })
                ) {
                  event.preventDefault();
                  void handleCreateProject();
                }
              }}
              placeholder="例如：季度复盘"
              disabled={createProjectSubmitting}
            />
          </label>
          <ProjectFoldersEditor
            folders={draftProjectFolders.map((path) => ({
              path,
              isPrimary: isSameWorkspacePath(path, draftPrimaryFolder),
              onMakePrimary: (target: string) => setDraftPrimaryFolder(target),
              onRemove: handleRemoveDraftProjectFolder,
            }))}
            addDisabled={pickingWorkspace || createProjectSubmitting}
            emptyHint="可以先不选文件夹。项目会直接创建，之后随时能加。"
            onAddFolder={() => {
              void handleAddDraftProjectFolder();
            }}
          />
        </div>
      </Dialog>
      <Dialog
        open={Boolean(editingProjectId)}
        title="编辑项目"
        onOpenChange={(open) => {
          if (!open && !projectFolderBusyPath && !projectRenameSubmitting) {
            closeProjectEditor();
          }
        }}
        footer={
          <Button
            variant="primary"
            size="md"
            loading={projectRenameSubmitting}
            disabled={Boolean(projectFolderBusyPath)}
            onClick={() => {
              void handleFinishEditingProject();
            }}
          >
            完成
          </Button>
        }
      >
        <div className="create-project-dialog-body">
          <label className="create-project-dialog-field">
            <span className="create-project-dialog-label">项目名称</span>
            <input
              type="text"
              className="create-project-dialog-input"
              value={editingProjectName}
              onChange={(event) => setEditingProjectName(event.target.value)}
              onKeyDown={(event) => {
                if (
                  shouldSubmitProjectNameInputKey({
                    key: event.key,
                    isComposing: event.nativeEvent.isComposing,
                  })
                ) {
                  event.preventDefault();
                  void handleFinishEditingProject();
                }
              }}
              placeholder="例如：季度复盘"
              disabled={projectRenameSubmitting}
              autoFocus
            />
          </label>
          <ProjectFoldersEditor
            folders={
              editingProjectId ? projectFolderRows(editingProjectId) : []
            }
            addDisabled={
              pickingWorkspace ||
              Boolean(projectFolderBusyPath) ||
              projectRenameSubmitting
            }
            busyPath={projectFolderBusyPath}
            emptyHint="这个项目还没有文件夹。"
            onAddFolder={() => {
              if (editingProjectId) {
                void handleAddProjectFolder(editingProjectId);
              }
            }}
          />
        </div>
      </Dialog>
      <Dialog
        open={Boolean(pendingProjectDelete)}
        title={projectDeleteDialogTitle}
        onOpenChange={(open) => {
          if (!open && !projectDeleteSubmitting) {
            setPendingProjectDelete(null);
          }
        }}
        footer={
          <>
            <Button
              variant="default"
              size="md"
              disabled={projectDeleteSubmitting}
              onClick={() => setPendingProjectDelete(null)}
            >
              取消
            </Button>
            <Button
              variant="primary"
              size="md"
              loading={projectDeleteSubmitting}
              onClick={() => {
                void handleConfirmProjectDelete();
              }}
            >
              {projectDeleteConfirmLabel}
            </Button>
          </>
        }
      >
        <p className="project-delete-dialog-copy">{projectDeleteDialogBody}</p>
      </Dialog>
    </>
  );
}

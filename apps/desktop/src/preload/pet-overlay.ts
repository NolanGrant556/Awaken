import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import {
  PET_IPC_CHANNELS,
  type PetActivitySnapshot,
  type PetOverlayBootstrap,
  type PetOverlayBounds,
  type PetOverlayBridge,
  type PetNotificationActionRequest,
  type ScreenPoint,
} from "../shared/pets.js";

const bridge: PetOverlayBridge = {
  getBootstrap: () =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.petOverlayBootstrap) as Promise<PetOverlayBootstrap>,

  onBootstrapChanged: (callback: (value: PetOverlayBootstrap) => void) => {
    const handler = (_event: IpcRendererEvent, value: PetOverlayBootstrap) => callback(value);
    ipcRenderer.on(PET_IPC_CHANNELS.petOverlayBootstrap, handler);
    return () => ipcRenderer.removeListener(PET_IPC_CHANNELS.petOverlayBootstrap, handler);
  },

  onActivityChanged: (callback: (value: PetActivitySnapshot) => void) => {
    const handler = (_event: IpcRendererEvent, value: PetActivitySnapshot) => callback(value);
    ipcRenderer.on(PET_IPC_CHANNELS.petActivityChanged, handler);
    return () => ipcRenderer.removeListener(PET_IPC_CHANNELS.petActivityChanged, handler);
  },

  onMainWindowFocused: (callback: () => void) => {
    const handler = () => callback();
    ipcRenderer.on(PET_IPC_CHANNELS.petMainWindowFocused, handler);
    return () => ipcRenderer.removeListener(PET_IPC_CHANNELS.petMainWindowFocused, handler);
  },

  beginDrag: (point: ScreenPoint) => {
    ipcRenderer.send(PET_IPC_CHANNELS.petDragStart, point);
  },

  moveDrag: (point: ScreenPoint) => {
    ipcRenderer.send(PET_IPC_CHANNELS.petDragMove, point);
  },

  endDrag: (point: ScreenPoint) => {
    ipcRenderer.send(PET_IPC_CHANNELS.petDragEnd, point);
  },

  setPointerInteractive: (interactive: boolean) => {
    ipcRenderer.send(PET_IPC_CHANNELS.petPointerInteractive, interactive);
  },

  resizeMascot: (width: number) => {
    ipcRenderer.send(PET_IPC_CHANNELS.petResizeMascot, width);
  },

  finishResize: (width: number) => {
    ipcRenderer.send(PET_IPC_CHANNELS.petFinishResize, width);
  },

  reportBounds: (bounds: PetOverlayBounds) => {
    ipcRenderer.send(PET_IPC_CHANNELS.petReportBounds, bounds);
  },

  activateNotification: (notificationId: string) =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.petActivateNotification, notificationId) as Promise<void>,

  performNotificationAction: (input: PetNotificationActionRequest) =>
    ipcRenderer.invoke(
      PET_IPC_CHANNELS.petPerformNotificationAction,
      input,
    ) as Promise<void>,

  dismissNotification: (input: { notificationId: string; turnKey: string | null }) =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.petDismissNotification, input) as Promise<void>,

  recordPetWelcomed: (petId: string) =>
    ipcRenderer.invoke(PET_IPC_CHANNELS.petRecordPetWelcomed, petId) as Promise<void>,

  openMainWindow: () => {
    ipcRenderer.send(PET_IPC_CHANNELS.petOpenMainWindow);
  },

  closeOverlay: () => {
    ipcRenderer.send(PET_IPC_CHANNELS.petCloseOverlay);
  },
};

contextBridge.exposeInMainWorld("awakenPetOverlay", bridge);

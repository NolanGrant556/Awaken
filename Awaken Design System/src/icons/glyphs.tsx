import type { SVGProps } from "react";
import { AwakenIcon } from "../components/AwakenIcon/AwakenIcon";
import { awakenLucideClassName } from "./lucideClass";
import { AWAKEN_LUCIDE_GLYPH_IDS, type AwakenIconId, type AwakenLucideGlyphName } from "./types";

export type AwakenGlyphProps = SVGProps<SVGSVGElement> & {
  size?: number | string;
  color?: string;
  strokeWidth?: number | string;
  absoluteStrokeWidth?: boolean;
};

export function createAwakenGlyph(exportName: AwakenLucideGlyphName) {
  const iconId = AWAKEN_LUCIDE_GLYPH_IDS[exportName] as AwakenIconId;
  const lucideClassName = awakenLucideClassName(exportName);
  const Glyph = ({
    size = 16,
    className,
    color,
    strokeWidth: _strokeWidth,
    absoluteStrokeWidth: _absoluteStrokeWidth,
    id: _htmlId,
    ...rest
  }: AwakenGlyphProps) => (
    <AwakenIcon
      {...rest}
      id={iconId}
      size={size}
      color={color}
      className={[iconId === "loader" ? "aw-icon--spin" : "", className].filter(Boolean).join(" ") || undefined}
      lucideClassName={lucideClassName}
    />
  );
  Glyph.displayName = exportName;
  return Glyph;
}

export type AwakenGlyphComponent = ReturnType<typeof createAwakenGlyph>;

export const AlertCircle = createAwakenGlyph("AlertCircle");
export const AlertTriangle = createAwakenGlyph("AlertTriangle");
export const AppWindow = createAwakenGlyph("AppWindow");
export const ArrowDownCircle = createAwakenGlyph("ArrowDownCircle");
export const ArrowLeft = createAwakenGlyph("ArrowLeft");
export const ArrowRight = createAwakenGlyph("ArrowRight");
export const ArrowUp = createAwakenGlyph("ArrowUp");
export const ArrowUpRight = createAwakenGlyph("ArrowUpRight");
export const Ban = createAwakenGlyph("Ban");
export const BookOpenText = createAwakenGlyph("BookOpenText");
export const Bot = createAwakenGlyph("Bot");
export const Brain = createAwakenGlyph("Brain");
export const CalendarClock = createAwakenGlyph("CalendarClock");
export const CalendarDays = createAwakenGlyph("CalendarDays");
export const Check = createAwakenGlyph("Check");
export const CheckCheck = createAwakenGlyph("CheckCheck");
export const CheckCircle2 = createAwakenGlyph("CheckCircle2");
export const ChevronDown = createAwakenGlyph("ChevronDown");
export const ChevronLeft = createAwakenGlyph("ChevronLeft");
export const ChevronRight = createAwakenGlyph("ChevronRight");
export const ChevronUp = createAwakenGlyph("ChevronUp");
export const ChevronsDownUp = createAwakenGlyph("ChevronsDownUp");
export const ChevronsUpDown = createAwakenGlyph("ChevronsUpDown");
export const Circle = createAwakenGlyph("Circle");
export const CircleAlert = createAwakenGlyph("CircleAlert");
export const CircleCheck = createAwakenGlyph("CircleCheck");
export const CircleSlash = createAwakenGlyph("CircleSlash");
export const CircleX = createAwakenGlyph("CircleX");
export const Clock = createAwakenGlyph("Clock");
export const Clock3 = createAwakenGlyph("Clock3");
export const Code = createAwakenGlyph("Code");
export const Copy = createAwakenGlyph("Copy");
export const Cpu = createAwakenGlyph("Cpu");
export const ExternalLink = createAwakenGlyph("ExternalLink");
export const Eye = createAwakenGlyph("Eye");
export const EyeOff = createAwakenGlyph("EyeOff");
export const File = createAwakenGlyph("File");
export const FileArchive = createAwakenGlyph("FileArchive");
export const FileAudio = createAwakenGlyph("FileAudio");
export const FileAudio2 = createAwakenGlyph("FileAudio2");
export const FileCode = createAwakenGlyph("FileCode");
export const FileCode2 = createAwakenGlyph("FileCode2");
export const FileImage = createAwakenGlyph("FileImage");
export const FileJson = createAwakenGlyph("FileJson");
export const FileJson2 = createAwakenGlyph("FileJson2");
export const FileSpreadsheet = createAwakenGlyph("FileSpreadsheet");
export const FileText = createAwakenGlyph("FileText");
export const FileType = createAwakenGlyph("FileType");
export const FileType2 = createAwakenGlyph("FileType2");
export const FileUp = createAwakenGlyph("FileUp");
export const FileVideo = createAwakenGlyph("FileVideo");
export const FileVideo2 = createAwakenGlyph("FileVideo2");
export const Folder = createAwakenGlyph("Folder");
export const FolderOpen = createAwakenGlyph("FolderOpen");
export const FolderPlus = createAwakenGlyph("FolderPlus");
export const FolderSearch = createAwakenGlyph("FolderSearch");
export const FolderSearch2 = createAwakenGlyph("FolderSearch2");
export const Folders = createAwakenGlyph("Folders");
export const Gauge = createAwakenGlyph("Gauge");
export const Globe = createAwakenGlyph("Globe");
export const Globe2 = createAwakenGlyph("Globe2");
export const History = createAwakenGlyph("History");
export const Image = createAwakenGlyph("Image");
export const KeyRound = createAwakenGlyph("KeyRound");
export const Link = createAwakenGlyph("Link");
export const Link2 = createAwakenGlyph("Link2");
export const ListChecks = createAwakenGlyph("ListChecks");
export const ListFilter = createAwakenGlyph("ListFilter");
export const Loader = createAwakenGlyph("Loader");
export const Loader2 = createAwakenGlyph("Loader2");
export const LoaderCircle = createAwakenGlyph("LoaderCircle");
export const Maximize = createAwakenGlyph("Maximize");
export const Maximize2 = createAwakenGlyph("Maximize2");
export const MessageCircle = createAwakenGlyph("MessageCircle");
export const MessageCirclePlus = createAwakenGlyph("MessageCirclePlus");
export const MessageSquare = createAwakenGlyph("MessageSquare");
export const Minimize = createAwakenGlyph("Minimize");
export const Minimize2 = createAwakenGlyph("Minimize2");
export const Minus = createAwakenGlyph("Minus");
export const Monitor = createAwakenGlyph("Monitor");
export const MonitorCog = createAwakenGlyph("MonitorCog");
export const Moon = createAwakenGlyph("Moon");
export const MoreHorizontal = createAwakenGlyph("MoreHorizontal");
export const OctagonX = createAwakenGlyph("OctagonX");
export const Package = createAwakenGlyph("Package");
export const Palette = createAwakenGlyph("Palette");
export const PanelLeft = createAwakenGlyph("PanelLeft");
export const PanelRightClose = createAwakenGlyph("PanelRightClose");
export const PanelRightOpen = createAwakenGlyph("PanelRightOpen");
export const PawPrint = createAwakenGlyph("PawPrint");
export const Pencil = createAwakenGlyph("Pencil");
export const Play = createAwakenGlyph("Play");
export const Plug = createAwakenGlyph("Plug");
export const PlugZap = createAwakenGlyph("PlugZap");
export const Plus = createAwakenGlyph("Plus");
export const Power = createAwakenGlyph("Power");
export const Presentation = createAwakenGlyph("Presentation");
export const Refresh = createAwakenGlyph("Refresh");
export const RefreshCw = createAwakenGlyph("RefreshCw");
export const RotateCcw = createAwakenGlyph("RotateCcw");
export const RotateCw = createAwakenGlyph("RotateCw");
export const Save = createAwakenGlyph("Save");
export const Scan = createAwakenGlyph("Scan");
export const Search = createAwakenGlyph("Search");
export const Server = createAwakenGlyph("Server");
export const Settings = createAwakenGlyph("Settings");
export const ShieldCheck = createAwakenGlyph("ShieldCheck");
export const ShieldOff = createAwakenGlyph("ShieldOff");
export const Square = createAwakenGlyph("Square");
export const Sun = createAwakenGlyph("Sun");
export const Terminal = createAwakenGlyph("Terminal");
export const TerminalSquare = createAwakenGlyph("TerminalSquare");
export const Trash = createAwakenGlyph("Trash");
export const Trash2 = createAwakenGlyph("Trash2");
export const TriangleAlert = createAwakenGlyph("TriangleAlert");
export const Upload = createAwakenGlyph("Upload");
export const UserRound = createAwakenGlyph("UserRound");
export const Video = createAwakenGlyph("Video");
export const Wallet = createAwakenGlyph("Wallet");
export const Webhook = createAwakenGlyph("Webhook");
export const Wifi = createAwakenGlyph("Wifi");
export const Workflow = createAwakenGlyph("Workflow");
export const WrapText = createAwakenGlyph("WrapText");
export const Wrench = createAwakenGlyph("Wrench");
export const X = createAwakenGlyph("X");
export const Zap = createAwakenGlyph("Zap");
export const ZoomIn = createAwakenGlyph("ZoomIn");
export const ZoomOut = createAwakenGlyph("ZoomOut");

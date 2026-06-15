export type ConversationItem = {
  id: string;
  label: string;
  timeLabel: string;
};

export type ProjectItem = {
  id: string;
  label: string;
  timeLabel?: string;
  children?: ProjectItem[];
  isLoading?: boolean;
};

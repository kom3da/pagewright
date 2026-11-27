import { Badge, Group } from "@mantine/core";
import { CalendarClock, CircleAlert, CircleCheck, CircleDashed, Mail, PenLine } from "lucide-react";
import type { ContentStatus } from "../../api/types";

/**
 * Every status the admin shows, with one fixed color and icon each. The text is always
 * shown: color and icon only repeat it (WCAG 1.4.1).
 */
const statuses = {
  draft: { label: "下書き", color: "gray", icon: CircleDashed },
  scheduled: { label: "予約", color: "orange", icon: CalendarClock },
  published: { label: "公開中", color: "green", icon: CircleCheck },
  unpublishedChanges: { label: "未公開の変更あり", color: "brand", icon: PenLine },
  scheduledUpdate: { label: "更新を予約", color: "orange", icon: CalendarClock },
  // Contact messages.
  unhandled: { label: "未対応", color: "orange", icon: CircleAlert },
  handled: { label: "対応済み", color: "green", icon: CircleCheck },
  // Invitations.
  invited: { label: "返事待ち", color: "gray", icon: Mail },
  invitationExpired: { label: "期限切れ", color: "orange", icon: CircleAlert },
} as const;

export type StatusKind = keyof typeof statuses;

export function StatusBadge({ status }: { status: StatusKind }) {
  const { label, color, icon: Icon } = statuses[status];
  return (
    <Badge color={color} leftSection={<Icon aria-hidden="true" size={13} strokeWidth={2.5} />}>
      {label}
    </Badge>
  );
}

type ContentLike = {
  status: ContentStatus;
  scheduledAt: string | null;
  hasUnpublishedChanges: boolean;
  /** Only known in the editor; when it equals the current revision, the changes are scheduled. */
  scheduledRevisionId?: number | null;
  revisionId?: number;
};

/** The badges of a post or page, in the same order everywhere (list and editor). */
export function contentStatuses(c: ContentLike): StatusKind[] {
  const out: StatusKind[] = [c.status];
  if (c.status !== "published") return out;
  if (c.scheduledAt) out.push("scheduledUpdate");
  // With a scheduled update the list cannot tell whether the changes are part of it, so
  // only the schedule is shown there. The editor knows (scheduledRevisionId).
  const changesScheduled =
    c.revisionId !== undefined ? c.scheduledRevisionId === c.revisionId : c.scheduledAt !== null;
  if (c.hasUnpublishedChanges && !changesScheduled) out.push("unpublishedChanges");
  return out;
}

export function ContentStatusBadges(props: ContentLike) {
  return (
    <Group gap={6} wrap="wrap">
      {contentStatuses(props).map((s) => (
        <StatusBadge key={s} status={s} />
      ))}
    </Group>
  );
}

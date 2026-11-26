import { Anchor, type AnchorProps, VisuallyHidden } from "@mantine/core";
import { ExternalLink as ExternalIcon } from "lucide-react";
import type { ReactNode } from "react";

/**
 * A link that opens in a new tab (the public site, help pages). Shows an icon and tells
 * screen reader users that a new tab opens. In-app links use
 * `<AnchorLink to="…">` instead.
 */
export function ExternalLink({
  href,
  children,
  ...props
}: AnchorProps & { href: string; children: ReactNode }) {
  return (
    <Anchor href={href} target="_blank" rel="noopener noreferrer" {...props}>
      {children}
      <ExternalIcon
        aria-hidden="true"
        size="0.9em"
        style={{ marginInlineStart: 4, verticalAlign: "-0.1em" }}
      />
      <VisuallyHidden>（新しいタブで開きます）</VisuallyHidden>
    </Anchor>
  );
}

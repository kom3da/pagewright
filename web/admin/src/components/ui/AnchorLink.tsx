import { Anchor, type AnchorProps } from "@mantine/core";
import { createLink, type LinkComponent } from "@tanstack/react-router";
import type { ComponentProps } from "react";

type MantineAnchorProps = AnchorProps & Omit<ComponentProps<"a">, keyof AnchorProps>;

function MantineAnchor(props: MantineAnchorProps) {
  return <Anchor {...props} />;
}

const CreatedAnchorLink = createLink(MantineAnchor);

/**
 * An in-app text link: Mantine's Anchor look with TanStack Router's typed `to` / `params`.
 *
 *   <AnchorLink to="/posts/$id" params={{ id: "1" }}>記事を開く</AnchorLink>
 */
export const AnchorLink: LinkComponent<typeof MantineAnchor> = (props) => (
  <CreatedAnchorLink {...props} />
);

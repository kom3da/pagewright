import { Button } from "@mantine/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { api, errorMessage } from "../api/client";
import type { ContentType } from "../api/types";
import { notify } from "./ui/notify";

/** Creates an empty post/page and opens it in the editor. */
export function NewContentButton({
  type,
  label,
  primary,
}: {
  type: ContentType;
  label: string;
  /** The screen's primary action (filled). Only one per screen. */
  primary?: boolean;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const create = useMutation({
    mutationFn: () => api.createContent(type),
    onSuccess: (content) => {
      queryClient.setQueryData(["content", content.id], content);
      void queryClient.invalidateQueries({ queryKey: ["contents", type] });
      void navigate({
        to: type === "post" ? "/posts/$id" : "/pages/$id",
        params: { id: String(content.id) },
      });
    },
    onError: (e) => notify(errorMessage(e), "error"),
  });
  return (
    <Button
      variant={primary ? "filled" : "default"}
      leftSection={<Plus aria-hidden="true" size={18} />}
      onClick={() => create.mutate()}
      loading={create.isPending}
    >
      {label}
    </Button>
  );
}

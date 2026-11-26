import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { api, errorMessage } from "../api/client";
import type { ContentType } from "../api/types";
import { useNotify } from "./Toaster";
import { Button } from "./ui/Button";

/** Creates an empty post/page and opens it in the editor. */
export function NewContentButton({ type, label }: { type: ContentType; label: string }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const notify = useNotify();
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
    <Button variant="primary" onClick={() => create.mutate()} disabled={create.isPending}>
      <Plus aria-hidden="true" size={18} />
      {label}
    </Button>
  );
}

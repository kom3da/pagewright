import {
  Button,
  Group,
  Image,
  Paper,
  SimpleGrid,
  Stack,
  Text,
  Textarea,
  Title,
  UnstyledButton,
} from "@mantine/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Editor } from "@tiptap/react";
import { type FormEvent, useRef, useState } from "react";
import { api, errorField, errorMessage } from "../api/client";
import { mediaQuery } from "../api/queries";
import type { Media } from "../api/types";
import { confirm } from "../components/ui/confirm";
import { Dialog } from "../components/ui/Dialog";
import { Field } from "../components/ui/Field";
import { Form } from "../components/ui/Form";
import { ErrorMessage, Loading } from "../components/ui/feedback";
import { notify } from "../components/ui/notify";
import classes from "./MediaDialog.module.css";

type Props = { editor: Editor; open: boolean; onClose: () => void };

const MAX_BYTES = 10 * 1024 * 1024;
const ACCEPT = ["image/jpeg", "image/png", "image/gif", "image/webp"];
const altHint =
  "画像の内容を短く説明してください。目の不自由な方の読み上げソフトや、画像が表示されないときに使われます。例：会社の外観";

export function MediaDialog({ editor, open, onClose }: Props) {
  const insert = (media: Media, alt: string) => {
    editor.chain().focus().insertImage({ src: media.url, alt, mediaId: media.id }).run();
    onClose();
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title="画像を入れる"
      wide
      onCloseFocus={() => editor.commands.focus()}
    >
      {open && (
        <Stack gap="xl">
          <UploadForm onUploaded={insert} />
          <MediaPicker onPick={insert} inBody={(id) => bodyUsesMedia(editor, id)} />
        </Stack>
      )}
    </Dialog>
  );
}

export function UploadForm({
  onUploaded,
  defaultAlt = "",
  submitLabel = "アップロードして入れる",
}: {
  onUploaded: (m: Media, alt: string) => void;
  defaultAlt?: string;
  submitLabel?: string;
}) {
  const queryClient = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [alt, setAlt] = useState(defaultAlt);
  const [errors, setErrors] = useState<{ file?: string; alt?: string }>({});
  const upload = useMutation({
    mutationFn: (v: { file: File; alt: string }) => api.uploadMedia(v.file, v.alt),
    onSuccess: (media, v) => {
      void queryClient.invalidateQueries({ queryKey: mediaQuery.queryKey });
      onUploaded(media, v.alt);
    },
    onError: (e) => {
      const field = errorField(e);
      if (field === "file") {
        setErrors({ file: errorMessage(e) });
        fileRef.current?.focus();
      } else if (field === "altText") {
        setErrors({ alt: errorMessage(e) });
        altRef.current?.focus();
      }
    },
  });
  const fileRef = useRef<HTMLInputElement>(null);
  const altRef = useRef<HTMLTextAreaElement>(null);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const next: typeof errors = {};
    if (!file) next.file = "画像ファイルを選んでください。";
    else if (!ACCEPT.includes(file.type))
      next.file = "使える画像の形式は JPEG・PNG・GIF・WebP です。";
    else if (file.size > MAX_BYTES) next.file = "10MBまでの画像を選んでください。";
    if (!alt.trim()) next.alt = "画像の説明（代替テキスト）を入力してください。";
    setErrors(next);
    if (file && !next.file && !next.alt) upload.mutate({ file, alt: alt.trim() });
  };

  return (
    <Stack component="section" aria-labelledby="upload-heading" gap="sm">
      <Title order={3} id="upload-heading">
        新しい画像をアップロード
      </Title>
      <Form onSubmit={onSubmit} gap="md">
        <Field
          label="画像ファイル"
          description="JPEG・PNG・GIF・WebP、10MBまで"
          error={errors.file}
          required
        >
          {(ids) => (
            <input
              {...ids}
              ref={fileRef}
              type="file"
              accept={ACCEPT.join(",")}
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              className={classes.file}
            />
          )}
        </Field>
        <Textarea
          ref={altRef}
          label="画像の説明（代替テキスト）"
          description={altHint}
          error={errors.alt}
          required
          value={alt}
          onChange={(e) => setAlt(e.currentTarget.value)}
        />
        {upload.isError && !errorField(upload.error) && (
          <ErrorMessage>{errorMessage(upload.error)}</ErrorMessage>
        )}
        <Group>
          <Button type="submit" variant="filled" loading={upload.isPending}>
            {submitLabel}
          </Button>
        </Group>
      </Form>
    </Stack>
  );
}

/** Whether the (possibly unsaved) body in the editor shows the image. */
function bodyUsesMedia(editor: Editor, mediaId: number) {
  let found = false;
  editor.state.doc.descendants((node) => {
    if (node.type.name === "image" && node.attrs.mediaId === mediaId) found = true;
    return !found;
  });
  return found;
}

export function MediaPicker({
  onPick,
  inBody,
  withAlt = true,
  pickLabel = "この画像を入れる",
  inUseMessage = "この画像は編集中の本文で使われているため削除できません。先に本文から外してください。",
}: {
  onPick: (m: Media, alt: string) => void;
  /** Whether the image is in use by what is being edited, so it must not be deleted. */
  inBody: (mediaId: number) => boolean;
  /** Ask for the alt text when picking (not needed for the logo, whose alt is the site name). */
  withAlt?: boolean;
  pickLabel?: string;
  inUseMessage?: string;
}) {
  const queryClient = useQueryClient();
  const { data, isPending, error } = useQuery(mediaQuery);
  const [selected, setSelected] = useState<Media | null>(null);
  const [alt, setAlt] = useState("");
  const [altError, setAltError] = useState<string>();
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const choose = (m: Media) => {
    setSelected(m);
    setAlt(m.altText);
    setAltError(undefined);
    setDeleteError(null);
  };

  const remove = useMutation({
    mutationFn: (m: Media) => api.deleteMedia(m.id),
    onSuccess: (_, m) => {
      setSelected(null);
      void queryClient.invalidateQueries({ queryKey: mediaQuery.queryKey });
      notify(`画像「${m.altText || m.filename}」を削除しました。`);
    },
    onError: (e) => {
      // 409 media_in_use: the message tells the user to remove it from the body first.
      setDeleteError(errorMessage(e));
    },
  });

  const askDelete = async () => {
    if (!selected) return;
    if (inBody(selected.id)) {
      setDeleteError(inUseMessage);
      return;
    }
    setDeleteError(null);
    const ok = await confirm({
      title: "この画像を削除しますか？",
      message: `「${selected.altText || selected.filename}」を削除します。元に戻せません。記事やページで使っている画像は削除できません。`,
      confirmLabel: "削除する",
      danger: true,
    });
    if (ok) remove.mutate(selected);
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!selected) return;
    if (withAlt && !alt.trim()) {
      setAltError("画像の説明（代替テキスト）を入力してください。");
      return;
    }
    onPick(selected, alt.trim());
  };

  return (
    <Stack component="section" aria-labelledby="library-heading" gap="sm">
      <Title order={3} id="library-heading">
        アップロード済みの画像から選ぶ
      </Title>
      {isPending ? (
        <Loading />
      ) : error ? (
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      ) : data.length === 0 ? (
        <Text c="dimmed">まだ画像がありません。</Text>
      ) : (
        <SimpleGrid component="ul" cols={{ base: 2, sm: 4 }} spacing="sm" className={classes.grid}>
          {data.map((m) => (
            <li key={m.id}>
              <UnstyledButton
                aria-pressed={selected?.id === m.id}
                onClick={() => choose(m)}
                className={classes.tile}
              >
                <Image
                  src={m.previewUrl}
                  alt=""
                  radius="xs"
                  className={classes.thumb}
                  loading="lazy"
                />
                <Text size="sm" mt={4} truncate>
                  {m.altText || m.filename}
                </Text>
              </UnstyledButton>
            </li>
          ))}
        </SimpleGrid>
      )}
      {selected && (
        <Paper bg="gray.0" p="md">
          <Form onSubmit={onSubmit} gap="md">
            {withAlt && (
              <Textarea
                label="画像の説明（代替テキスト）"
                description={altHint}
                error={altError}
                required
                value={alt}
                onChange={(e) => setAlt(e.currentTarget.value)}
              />
            )}
            {deleteError && <ErrorMessage>{deleteError}</ErrorMessage>}
            <Group gap="sm">
              <Button type="submit" variant="filled">
                {pickLabel}
              </Button>
              <Button variant="subtle" color="red" ml="auto" onClick={askDelete}>
                この画像を削除
              </Button>
            </Group>
          </Form>
        </Paper>
      )}
    </Stack>
  );
}

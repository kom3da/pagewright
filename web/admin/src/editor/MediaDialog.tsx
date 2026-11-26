import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Editor } from "@tiptap/react";
import { clsx } from "clsx";
import { type FormEvent, useRef, useState } from "react";
import { api, errorField, errorMessage } from "../api/client";
import { mediaQuery } from "../api/queries";
import type { Media } from "../api/types";
import { Button } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { Field, inputClass } from "../components/ui/Field";
import { ErrorMessage, Loading } from "../components/ui/misc";

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
        <div className="space-y-8">
          <UploadForm onUploaded={insert} />
          <MediaPicker onPick={insert} />
        </div>
      )}
    </Dialog>
  );
}

function UploadForm({ onUploaded }: { onUploaded: (m: Media, alt: string) => void }) {
  const queryClient = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [alt, setAlt] = useState("");
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
    <section aria-labelledby="upload-heading" className="space-y-3">
      <h3 id="upload-heading" className="text-lg font-bold">
        新しい画像をアップロード
      </h3>
      <form noValidate onSubmit={onSubmit} className="space-y-4">
        <Field
          label="画像ファイル"
          hint="JPEG・PNG・GIF・WebP、10MBまで"
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
              className="block w-full text-sm file:mr-3 file:rounded-md file:border file:border-slate-500 file:bg-white file:px-3 file:py-2 file:font-semibold"
            />
          )}
        </Field>
        <Field label="画像の説明（代替テキスト）" hint={altHint} error={errors.alt} required>
          {(ids) => (
            <textarea
              {...ids}
              ref={altRef}
              rows={2}
              value={alt}
              onChange={(e) => setAlt(e.target.value)}
              className={inputClass}
            />
          )}
        </Field>
        {upload.isError && !errorField(upload.error) && (
          <ErrorMessage>{errorMessage(upload.error)}</ErrorMessage>
        )}
        <Button type="submit" variant="primary" disabled={upload.isPending}>
          {upload.isPending ? "アップロードしています…" : "アップロードして入れる"}
        </Button>
      </form>
    </section>
  );
}

function MediaPicker({ onPick }: { onPick: (m: Media, alt: string) => void }) {
  const { data, isPending, error } = useQuery(mediaQuery);
  const [selected, setSelected] = useState<Media | null>(null);
  const [alt, setAlt] = useState("");
  const [altError, setAltError] = useState<string>();

  const choose = (m: Media) => {
    setSelected(m);
    setAlt(m.altText);
    setAltError(undefined);
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!selected) return;
    if (!alt.trim()) {
      setAltError("画像の説明（代替テキスト）を入力してください。");
      return;
    }
    onPick(selected, alt.trim());
  };

  return (
    <section aria-labelledby="library-heading" className="space-y-3">
      <h3 id="library-heading" className="text-lg font-bold">
        アップロード済みの画像から選ぶ
      </h3>
      {isPending ? (
        <Loading />
      ) : error ? (
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      ) : data.length === 0 ? (
        <p>まだ画像がありません。</p>
      ) : (
        <ul className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {data.map((m) => (
            <li key={m.id}>
              <button
                type="button"
                aria-pressed={selected?.id === m.id}
                onClick={() => choose(m)}
                className={clsx(
                  "block w-full rounded-md border-2 p-1 text-left",
                  selected?.id === m.id ? "border-blue-700 bg-blue-50" : "border-slate-300",
                )}
              >
                <img
                  src={m.previewUrl}
                  alt=""
                  className="aspect-square w-full rounded object-cover"
                  loading="lazy"
                />
                <span className="mt-1 block truncate text-sm">{m.altText || m.filename}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {selected && (
        <form noValidate onSubmit={onSubmit} className="space-y-4 rounded-md bg-slate-100 p-4">
          <Field label="画像の説明（代替テキスト）" hint={altHint} error={altError} required>
            {(ids) => (
              <textarea
                {...ids}
                rows={2}
                value={alt}
                onChange={(e) => setAlt(e.target.value)}
                className={inputClass}
              />
            )}
          </Field>
          <Button type="submit" variant="primary">
            この画像を入れる
          </Button>
        </form>
      )}
    </section>
  );
}

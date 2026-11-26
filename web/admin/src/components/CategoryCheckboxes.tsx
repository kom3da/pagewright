import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { errorMessage } from "../api/client";
import { categoriesQuery } from "../api/queries";
import { buildCategoryTree, type CategoryNode } from "../lib/categoryTree";
import { ErrorMessage, Loading } from "./ui/misc";

type Props = { value: number[]; onChange: (ids: number[]) => void };

export function CategoryCheckboxes({ value, onChange }: Props) {
  const { data, isPending, error } = useQuery(categoriesQuery);
  const toggle = (id: number, checked: boolean) =>
    onChange(checked ? [...value, id] : value.filter((v) => v !== id));

  const renderNodes = (nodes: CategoryNode[]) => (
    <ul className="space-y-1">
      {nodes.map((n) => (
        <li key={n.id}>
          <label className="inline-flex items-center gap-2">
            <input
              type="checkbox"
              className="size-5"
              checked={value.includes(n.id)}
              onChange={(e) => toggle(n.id, e.target.checked)}
            />
            {n.name}
          </label>
          {n.children.length > 0 && <div className="ml-7">{renderNodes(n.children)}</div>}
        </li>
      ))}
    </ul>
  );

  return (
    <fieldset className="space-y-2">
      <legend className="font-semibold">カテゴリ</legend>
      {isPending ? (
        <Loading />
      ) : error ? (
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      ) : data.length === 0 ? (
        <p className="text-sm text-slate-700">
          カテゴリがまだありません。
          <Link to="/categories" className="text-blue-700 underline">
            カテゴリの画面
          </Link>
          で作れます。
        </p>
      ) : (
        renderNodes(buildCategoryTree(data))
      )}
    </fieldset>
  );
}

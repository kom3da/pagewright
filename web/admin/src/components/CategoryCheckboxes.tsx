import { Box, Checkbox, Fieldset, Stack, Text } from "@mantine/core";
import { useQuery } from "@tanstack/react-query";
import { errorMessage } from "../api/client";
import { categoriesQuery } from "../api/queries";
import { buildCategoryTree, type CategoryNode } from "../lib/categoryTree";
import choice from "./ChoiceRow.module.css";
import { AnchorLink } from "./ui/AnchorLink";
import { ErrorMessage, Loading } from "./ui/feedback";

type Props = { value: number[]; onChange: (ids: number[]) => void };

export function CategoryCheckboxes({ value, onChange }: Props) {
  const { data, isPending, error } = useQuery(categoriesQuery);
  const toggle = (id: number, checked: boolean) =>
    onChange(checked ? [...value, id] : value.filter((v) => v !== id));

  const renderNodes = (nodes: CategoryNode[]) => (
    <Stack component="ul" gap={0} m={0} p={0} style={{ listStyle: "none" }}>
      {nodes.map((n) => (
        <li key={n.id}>
          <Checkbox
            label={n.name}
            checked={value.includes(n.id)}
            onChange={(e) => toggle(n.id, e.currentTarget.checked)}
            classNames={choice}
          />
          {n.children.length > 0 && <Box ml={28}>{renderNodes(n.children)}</Box>}
        </li>
      ))}
    </Stack>
  );

  return (
    <Fieldset legend="カテゴリ" variant="unstyled">
      {isPending ? (
        <Loading />
      ) : error ? (
        <ErrorMessage>{errorMessage(error)}</ErrorMessage>
      ) : data.length === 0 ? (
        <Text size="sm" c="dimmed">
          カテゴリがまだありません。
          <AnchorLink to="/categories" inherit>
            カテゴリの画面
          </AnchorLink>
          で作れます。
        </Text>
      ) : (
        renderNodes(buildCategoryTree(data))
      )}
    </Fieldset>
  );
}

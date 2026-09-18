import { GraphQLClient } from "graphql-request";
import { useAuthStore } from "@/store/auth.store";

// Creates a GraphQL client with the current access token. Recreate by calling
// getGQLClient() — it reads the token fresh each time.
export function getGQLClient() {
  const { accessToken } = useAuthStore.getState();

  return new GraphQLClient("/api/graphql", {
    headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : {},
  });
}

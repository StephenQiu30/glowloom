type ProviderNameSource = Pick<API.CreatedProvider, "name">;

/** Hide the generated short hexadecimal ID when appended to a connection name. */
export function providerConnectionName(provider?: ProviderNameSource) {
  const name = provider?.name?.trim();
  return name?.replace(/\s*·\s*[a-f0-9]{8}$/i, "");
}

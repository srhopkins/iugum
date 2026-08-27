// Perses 0.54 looks up plugins by "Kind:Name::version", not the short export name.
export function keyedPluginModule(mod, resource) {
  const out = { ...mod };
  const version = (resource.metadata && resource.metadata.version) || "";
  const registry = (resource.metadata && resource.metadata.registry) || "";
  const plugins = (resource.spec && resource.spec.plugins) || [];
  for (const plugin of plugins) {
    const name = plugin.spec && plugin.spec.name;
    if (!name) {
      continue;
    }
    const impl = mod[name];
    if (!impl) {
      continue;
    }
    out[`${plugin.kind}:${name}:${registry}:${version}`] = impl;
    out[name] = impl;
  }
  return out;
}

export function loadPlugin(mod) {
  const resource = mod.getPluginModule();
  return {
    resource,
    importPlugin: () => Promise.resolve(keyedPluginModule(mod, resource)),
  };
}

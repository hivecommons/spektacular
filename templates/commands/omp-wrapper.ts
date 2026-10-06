// Spektacular-owned wrapper. No imports or user settings are required.
export default () => ({
  name: {{{nameJSON}}},
  description: {{{descriptionJSON}}},
  execute(_args: string[], _ctx: unknown, rawArgs = "") {
    const instruction = {{{instructionJSON}}};
    return rawArgs.length ? instruction + "\n\n" + rawArgs : instruction;
  },
});

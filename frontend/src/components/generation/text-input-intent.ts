export type TextInputEdit = { from: number; to: number; text: string };
export type BeforeTextInput = {
  value: string;
  from: number;
  to: number;
  inputType: string;
};
function boundary(value: string, position: number) {
  return (
    position >= 0 &&
    position <= value.length &&
    !(
      position > 0 &&
      position < value.length &&
      /[\uD800-\uDBFF]/.test(value[position - 1]) &&
      /[\uDC00-\uDFFF]/.test(value[position])
    )
  );
}
/** Native beforeinput fixes the replaced range. This verifies that range;
 * it never searches a final-string diff to guess a character's contribution. */
export function nativeTextEdit(
  before: BeforeTextInput,
  after: string,
): TextInputEdit {
  let { from, to } = before;
  if (from === to && before.inputType.startsWith("delete")) {
    const removed = before.value.length - after.length;
    if (removed < 0) throw new Error("unsupported_text_input");
    if (before.inputType.endsWith("Backward")) from -= removed;
    else if (before.inputType.endsWith("Forward")) to += removed;
    else if (removed) throw new Error("unsupported_text_input");
  } else if (
    !before.inputType.startsWith("insert") &&
    !before.inputType.startsWith("delete")
  ) {
    throw new Error("unsupported_text_input");
  }
  const inserted = after.length - (before.value.length - (to - from));
  if (
    from > to ||
    inserted < 0 ||
    !boundary(before.value, from) ||
    !boundary(before.value, to)
  )
    throw new Error("unsupported_text_input");
  const text = after.slice(from, from + inserted);
  if (before.value.slice(0, from) + text + before.value.slice(to) !== after)
    throw new Error("unsupported_text_input");
  return { from, to, text };
}

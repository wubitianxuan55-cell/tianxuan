// User-message presentation helpers, kept pure so the transcript stays a thin
// renderer and the attachment-replacement rule is unit-tested.

const ATTACHMENT_RE = /@\.tianxuan\/attachments\/[^\s]+/g;

// displayUserText renders a user message's text for the bubble: attachment
// references (which are transport paths, not user content) collapse to a
// readable [image] placeholder.
export function displayUserText(text: string): string {
  return text.replace(ATTACHMENT_RE, "[image]");
}

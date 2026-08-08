import { describe, expect, it } from "vitest";
import { PROVIDER_PRESETS } from "./SettingsProviders";

describe("PROVIDER_PRESETS", () => {
  it("offers an OpenCode Zen preset wired to the opencode kind", () => {
    const zen = PROVIDER_PRESETS.find((p) => p.name === "opencode");
    expect(zen).toBeDefined();
    expect(zen?.kind).toBe("opencode");
    expect(zen?.baseUrl).toBe("https://opencode.ai/zen/v1");
    expect(zen?.models).toBe("deepseek-v4-flash-free");
    expect(zen?.apiEnv).toBe("OPENCODE_API_KEY");
  });
});

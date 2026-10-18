import { readdirSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { match as id } from "../params/id";
import { match as rubricid } from "../params/rubricid";
import { match as version } from "../params/version";

const routes = join(__dirname, "..", "routes");
const dirs = (p: string) => readdirSync(join(routes, p), { withFileTypes: true }).filter((e) => e.isDirectory()).map((e) => e.name).sort();

// The URL table of the old router, as the file routes spell it: params are checked by the matchers,
// and the route directories name them. The real router is exercised by the URL table e2e.
describe("route table", () => {
  it("has exactly the pages of the old router, each with its matchers", () => {
    expect(dirs(".")).toEqual(["[...rest]", "rubrics", "runs"]);
    expect(dirs("runs")).toEqual(["superpipeline"]);
    expect(dirs("runs/superpipeline")).toEqual(["[board=id]"]);
    expect(dirs("runs/superpipeline/[board=id]")).toEqual(["[run=id]"]);
    expect(dirs("rubrics")).toEqual(["[id=rubricid]"]);
    expect(dirs("rubrics/[id=rubricid]")).toEqual(["[version=version]"]);
  });
  it("accepts and rejects the segments router.test.ts did", () => {
    expect(id("brd_01") && id("run_01")).toBe(true);
    expect(rubricid("press") && version("2")).toBe(true);
    expect(id("brd 1")).toBe(false);
    expect(version("0")).toBe(false);
  });
});

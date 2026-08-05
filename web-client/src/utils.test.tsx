/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { render, screen } from "@testing-library/react";
import { appPath, autoLink, scrollToElement } from "./utils";

describe("appPath", () => {
  const publicUrl = process.env.PUBLIC_URL;

  // CRA types process.env as read-only, which is right everywhere but here.
  const servedFrom = (value: string) => {
    (process.env as { PUBLIC_URL?: string }).PUBLIC_URL = value;
  };

  afterEach(() => {
    servedFrom(publicUrl ?? "");
  });

  // The deployment almost everyone has. Nothing about it must change.
  it("leaves paths alone when GPM is served from the domain root", () => {
    servedFrom("");

    expect(appPath("/")).toBe("/");
    expect(appPath("/constraints/staging")).toBe("/constraints/staging");
  });

  it("prefixes paths with the subpath GPM is served from", () => {
    servedFrom("/gpm");

    expect(appPath("/")).toBe("/gpm/");
    expect(appPath("/logout")).toBe("/gpm/logout");
    expect(appPath("/constraints/staging#name")).toBe(
      "/gpm/constraints/staging#name",
    );
  });
});

describe("autoLink", () => {
  it("leaves text without links untouched", () => {
    render(
      <div data-testid="out">
        {autoLink("Pods must define a liveness probe")}
      </div>,
    );

    expect(screen.getByTestId("out")).toHaveTextContent(
      "Pods must define a liveness probe",
    );
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("turns a URL into a link and keeps the surrounding text", () => {
    render(
      <div data-testid="out">
        {autoLink("See https://example.com/policies for details")}
      </div>,
    );

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "https://example.com/policies");
    expect(link).toHaveTextContent("https://example.com/policies");
    expect(screen.getByTestId("out")).toHaveTextContent("See");
    expect(screen.getByTestId("out")).toHaveTextContent("for details");
  });

  // Violation messages routinely carry more than one link.
  it("links every URL in the text", () => {
    render(
      <div data-testid="out">
        {autoLink("Read https://example.com/a and http://example.com/b now")}
      </div>,
    );

    const hrefs = screen
      .getAllByRole("link")
      .map((a) => a.getAttribute("href"));
    expect(hrefs).toEqual(["https://example.com/a", "http://example.com/b"]);
  });

  // Links go to third-party docs, so they must not be able to reach back into GPM's window.
  it("opens links in a new tab without leaking the opener", () => {
    render(<div>{autoLink("https://example.com")}</div>);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noreferrer");
  });
});

describe("scrollToElement", () => {
  let scrollIntoView: jest.Mock;

  beforeEach(() => {
    jest.useFakeTimers();
    // jsdom does not implement scrollIntoView.
    scrollIntoView = jest.fn();
    Element.prototype.scrollIntoView = scrollIntoView;
  });

  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
    document.body.innerHTML = "";
  });

  it("scrolls to the element and highlights it, then removes the highlight", () => {
    document.body.innerHTML = `<div id="target"><span>constraint</span></div>`;

    scrollToElement("#target");

    const child = document.querySelector("#target span")!;
    expect(scrollIntoView).toHaveBeenCalledWith({ block: "start" });
    expect(child.classList.contains("highlighted")).toBe(true);

    jest.advanceTimersByTime(1000);
    expect(child.classList.contains("highlighted")).toBe(false);
  });

  it("scrolls smoothly when asked to", () => {
    document.body.innerHTML = `<div id="target"><span>constraint</span></div>`;

    scrollToElement("#target", true);

    expect(scrollIntoView).toHaveBeenCalledWith({
      behavior: "smooth",
      block: "start",
    });
  });

  // Kubernetes names and contexts contain dots and colons, which are CSS selector syntax.
  // They have to be escaped or the lookup throws instead of finding the element.
  it("finds elements whose id contains dots and colons", () => {
    document.body.innerHTML = `<div id="ns.default:pod"><span>x</span></div>`;

    expect(() => scrollToElement("#ns.default:pod")).not.toThrow();
    expect(scrollIntoView).toHaveBeenCalled();
  });

  it("does nothing when the element is not on the page", () => {
    document.body.innerHTML = `<div id="other"></div>`;

    expect(() => scrollToElement("#missing")).not.toThrow();
    expect(scrollIntoView).not.toHaveBeenCalled();
  });
});

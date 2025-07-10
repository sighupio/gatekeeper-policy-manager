/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export function scrollToElement(hash: string, smooth: boolean = false) {
  const element = document.querySelector(hash.replace(/:|\./g, '\\$&'));

  if (!element) {
    return;
  }

  element?.firstElementChild?.classList.toggle("highlighted");

  if (smooth) {
    element.scrollIntoView({ behavior: "smooth", block: "start" });
  } else {
    element.scrollIntoView({ block: "start" });
  }

  setTimeout(() => {
    element?.firstElementChild?.classList.toggle("highlighted");
  }, 1000);
}

export function autoLink(text: string) {
  const delimiter = /((?:https?:\/\/)(?:(?:[a-z0-9]?(?:[a-z0-9\-]{1,61}[a-z0-9])?\.[^\.|\s])+[a-z\.]*[a-z]+|(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(?:\.(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3})(?::\d{1,5})*[a-z0-9.,_\/~#&=;%+?\-\\(\\)]*)/gi;
  return (
    <>
      {text.split(delimiter).map(word => {
        const match = word.match(delimiter);
        if (match) {
          const url = match[0];
          return (
            <a href={url} target="_blank" rel="noreferrer">{url}</a>
          );
        }
        return word;
      })}
    </>
  );
};
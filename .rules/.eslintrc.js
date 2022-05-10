/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

module.exports = {
    extends: [
        "eslint:recommended",
        "plugin:react/recommended",
        "plugin:jsx-a11y/recommended",
        "plugin:react-hooks/recommended",
    ],
    parser: "babel-eslint", // Uses babel-eslint transforms.
    parserOptions: {
        "parserOptions": {
            "parser": "@typescript-eslint/parser",
            "project": "./app/web-client/tsconfig.json",
        },
    },
}

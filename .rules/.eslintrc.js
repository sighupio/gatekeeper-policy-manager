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

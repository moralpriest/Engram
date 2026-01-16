// Commitlint configuration for Engram
// https://commitlint.js.org/
// Install: npm install -g @commitlint/cli @commitlint/config-conventional

module.exports = {
  extends: ["@commitlint/config-conventional"],
  rules: {
    // Type must be one of these
    "type-enum": [
      2,
      "always",
      [
        "feat", // New feature
        "fix", // Bug fix
        "security", // Security fix (important for wallet)
        "docs", // Documentation
        "style", // Formatting, no code change
        "refactor", // Code change that neither fixes nor adds
        "perf", // Performance improvement
        "test", // Adding tests
        "build", // Build system or dependencies
        "ci", // CI configuration
        "chore", // Maintenance
        "revert", // Revert previous commit
      ],
    ],
    // Type is required
    "type-empty": [2, "never"],
    // Type must be lowercase
    "type-case": [2, "always", "lower-case"],
    // Subject is required
    "subject-empty": [2, "never"],
    // Subject max length
    "subject-max-length": [2, "always", 100],
    // No period at end of subject
    "subject-full-stop": [2, "never", "."],
    // Body max line length
    "body-max-line-length": [2, "always", 200],
    // Footer max line length
    "footer-max-line-length": [2, "always", 200],
  },
};

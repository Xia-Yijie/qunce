const path = require("path");

module.exports = {
  apps: [
    {
      name: "qunce",
      cwd: __dirname,
      script: path.join(__dirname, "bin", `qunce${process.platform === "win32" ? ".exe" : ""}`),
      interpreter: "none",
      env: {
        QUNCE_SERVER_HOST: "0.0.0.0",
        QUNCE_SERVER_PORT: "8000",
        QUNCE_SERVER_DATA_DIR: path.join(__dirname, ".qunce"),
      },
    },
  ],
};

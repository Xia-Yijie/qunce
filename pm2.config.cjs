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
        HTTP_PROXY: "http://127.0.0.1:7897",
        HTTPS_PROXY: "http://127.0.0.1:7897",
        ALL_PROXY: "socks5://127.0.0.1:7897",
        NO_PROXY: "127.0.0.1,localhost",
      },
    },
  ],
};

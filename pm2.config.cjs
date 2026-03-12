const path = require("path");

const root = __dirname;
const python =
  process.platform === "win32"
    ? path.join(root, ".pixi", "envs", "default", "python.exe")
    : path.join(root, ".pixi", "envs", "default", "bin", "python");

module.exports = {
  apps: [
    {
      name: "qunce-server",
      cwd: root,
      script: path.join(root, "server", "run_server.py"),
      interpreter: python,
      env: {
        QUNCE_SERVER_HOST: "0.0.0.0",
        QUNCE_SERVER_PORT: "8000",
        PYTHONPATH: root,
      },
    },
  ],
};

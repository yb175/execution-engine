import { describe, it, expect } from "vitest";
import request from "supertest";
import app from "./server.js";

describe("Broken Server Tests", () => {
  // Test 1: Server health check
  it("should return healthy status", async () => {
    const response = await request(app).get("/health");

    expect(response.status).toBe(200);
    expect(response.body).toBe("Server is healthy");
  });

  // Test 2: x and y should be integers
  it("should correctly add integer values", async () => {
    const response = await request(app)
      .post("/broken-route")
      .send({ x: "10", y: "20" });

    expect(Number(response.text)).toBe(30);
  });
});

/**
 * Custom TypeScript resolver for top movie recommendations.
 * Compiled on-the-fly via esbuild and executed inside Goja without Node.js.
 */
interface RecommendationContext {
  args: { limit?: number };
  couch: {
    find: (opts: any) => any[];
  };
}

export default function(ctx: RecommendationContext) {
  const limit = ctx.args.limit || 3;
  return ctx.couch.find({
    selector: { type: "movie", rating: { "$gte": 8.8 } },
    limit: limit,
  });
}

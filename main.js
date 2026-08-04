import Koa from "koa";
import bodyParser from "koa-bodyparser";

const app = new Koa();

let state = {};

app.use(bodyParser());

app.use(async ctx => {
    const method = ctx.request.method;
    if(method === "POST"){
      const key = ctx.request.body.state_key;
      const value = ctx.request.body.state_value;
      state[key] = value;
    }
    ctx.body = JSON.stringify(state);
});

app.listen(10000);

import express from "express" 

const app = express() ; 
app.use(express.json());

// Health route 
app.get("/health",(req,res)=>{
    res.json("Server is healthy") ;
})

// Expecting x and y to be number but the user is not passing as number 
app.post("/broken-route",(req,res)=>{
    const {x,y} = req.body ; 
    res.send(x+y) ; 
})

export default app ; 
app.listen(3000,()=>{
    console.log(`listning on PORT 3000`)
})
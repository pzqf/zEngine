# 脚本系统

## 简介
游戏中的战斗脚本。使用流程图graphml语言，用yed编辑。

## 使用方法
1. 初始化时注册所有函数
2. 初始化时加载所有的脚本数据（整个游戏使用到的）,(如果未加载，绑定脚本时也会加载)
3. 为相应的对像(holder)绑定脚本即可(BindScript)
4. 调用holder.Update执行状态机。
5. 任何对像，只需要组合ScriptHolder结构体即可使用。

## 注意事项
1. 每个脚本必须有一个Entry节点作为入口
2. 每个脚本必须有一个Exit节点作为出口
3. 边(edge)的条件执行返回值为false才会明显返回，否则就会选择一条执行且到达下一个节点。
4. 节本的执行内容不做返回值限制。

## 支持语法
1. 函数调用
2. 二元表达式
3. 一元表达式, PS:只支持！
4. 基本数据类型为，string, int, float64, uint8, bool
5. 基本数据类型计算，+,-,*,/,>,>=,==, !=, <, <=, &&, ||, ^,&,|,&^ , PS：不是所有类型都支持这些计算。
6. 可嵌套使用，例:fun(a()+1, "bbb", 1.234)
7. 注意返回值类型为基本数据类型，不支持返回error（以后可以考虑）,注册函数时需注意

## 其他
1. yed版本 3.22
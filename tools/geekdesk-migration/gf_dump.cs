// GeekDesk Data (BinaryFormatter) -> JSON 导出器。字段级反射，绕开属性 getter 的写盘副作用。
//
// 用法：
//   csc -nologo -out:gf_dump.exe ^
//       -r:"<GeekDesk 目录>\GeekDesk.exe" ^
//       -r:"<GeekDesk 目录>\lib\Newtonsoft.Json.dll" gf_dump.cs
//   gf_dump.exe "<GeekDesk 目录>\Data" geekdesk-data.json [<GeekDesk 目录>]
//
// 第三个参数可显式指定 GeekDesk 安装目录；省略时取 Data 所在目录的上一级。
using System;
using System.Collections.Generic;
using System.IO;
using System.Reflection;
using System.Runtime.Serialization.Formatters.Binary;
using Newtonsoft.Json;
using Newtonsoft.Json.Serialization;

class GfDump
{
    static string GEEK_DIR;
    static string LIB_DIR;

    static void Main(string[] args)
    {
        // 这里只能用字符串运算：Main 一旦引用 Newtonsoft.Json 的类型，JIT 会在钩子装上之前
        // 就去解析它，直接 FileLoadException。所以路径在装钩子之前算好，再进 RealMain。
        if (args.Length < 2)
        {
            Console.Error.WriteLine("usage: gf_dump <path to Data> <output.json> [geekdesk dir]");
            Environment.Exit(2);
        }
        string dataPath = Path.GetFullPath(args[0]);
        // Data 文件本身就在安装目录里，所以它的父目录就是 GeekDesk 目录。
        GEEK_DIR = args.Length >= 3 ? Path.GetFullPath(args[2]) : Path.GetDirectoryName(dataPath);
        LIB_DIR = Path.Combine(GEEK_DIR, "lib");

        AppDomain.CurrentDomain.AssemblyResolve += new ResolveEventHandler(Resolve);
        RealMain(dataPath, args[1]);
    }

    static void RealMain(string dataPath, string outputPath)
    {
        using (FileStream fs = File.OpenRead(dataPath))
        {
            BinaryFormatter fmt = new BinaryFormatter();
            object appData = fmt.Deserialize(fs);
            JsonSerializerSettings settings = new JsonSerializerSettings();
            settings.ContractResolver = new FieldResolver();
            settings.ReferenceLoopHandling = ReferenceLoopHandling.Ignore;
            settings.Formatting = Formatting.Indented;
            string json = JsonConvert.SerializeObject(appData, settings);
            File.WriteAllText(outputPath, json, new System.Text.UTF8Encoding(false));
        }
        Console.WriteLine("OK");
    }

    // BinaryFormatter 的对象状态 == 全部实例字段；只序列化字段就不会触发任何属性 getter。
    class FieldResolver : DefaultContractResolver
    {
        protected override IList<JsonProperty> CreateProperties(Type type, MemberSerialization ms)
        {
            List<JsonProperty> props = new List<JsonProperty>();
            FieldInfo[] fields = type.GetFields(BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Instance);
            // 稳定的字段顺序，重跑导出的 JSON 才可比对。
            Array.Sort(fields, (a, b) => string.CompareOrdinal(a.Name, b.Name));
            foreach (FieldInfo f in fields)
            {
                if (f.IsStatic) continue;
                if (f.IsInitOnly) continue;
                if (f.IsLiteral) continue;
                JsonProperty p = new JsonProperty();
                p.PropertyName = f.Name;
                p.Readable = true;
                p.Writable = true;
                p.PropertyType = f.FieldType;
                p.ValueProvider = new FieldValueProvider(f);
                props.Add(p);
            }
            return props;
        }
    }

    class FieldValueProvider : IValueProvider
    {
        FieldInfo f;
        public FieldValueProvider(FieldInfo f) { this.f = f; }
        public void SetValue(object target, object value) { f.SetValue(target, value); }
        public object GetValue(object target) { return f.GetValue(target); }
    }

    static Assembly Resolve(object sender, ResolveEventArgs e)
    {
        string name = new AssemblyName(e.Name).Name;
        string[] candidates = new string[] {
            Path.Combine(GEEK_DIR, name + ".dll"),
            Path.Combine(GEEK_DIR, name + ".exe"),
            Path.Combine(LIB_DIR, name + ".dll")
        };
        foreach (string p in candidates)
        {
            if (File.Exists(p)) return Assembly.LoadFrom(p);
        }
        return null;
    }
}
